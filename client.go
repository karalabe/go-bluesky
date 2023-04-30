// Copyright 2023 Péter Szilágyi. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/golang-jwt/jwt/v5"
)

var (
	// jwtAsyncRefreshThreshold is the remaining validity time of a JWT token
	// below which to trigger a session refresh on a background thread (i.e.
	// the client can still be actively used during).
	jwtAsyncRefreshThreshold = 5 * time.Minute

	// jwtSyncRefreshThreshold is the remaining validity time of a JWT token
	// below which to trigger a session refresh on a foreground thread (i.e.
	// the client blocks new API calls until the refresh finishes).
	jwtSyncRefreshThreshold = 2 * time.Minute
)

var (
	// ErrLoginUnauthorized is returned from a login attempt if the credentials
	// are rejected by the server or the local client (master credentials).
	ErrLoginUnauthorized = errors.New("unauthorized")

	// ErrMasterCredentials is returned from a login attempt if the credentials
	// are valid on the Bluesky server, but they are the user's master password.
	// Since that is a security malpractice, this library forbids it.
	ErrMasterCredentials = errors.New("master credentials used")

	// ErrSessionExpired is returned from any API call if the underlying session
	// has expired and a new login from scratch is required.
	ErrSessionExpired = errors.New("session expired")
)

// Client is an API client attached to (and authenticated to) a Bluesky PDS instance.
type Client struct {
	client *xrpc.Client // Underlying XRPC transport connected to the API

	jwtLock          sync.RWMutex                // Lock protecting the following JWT auth fields
	jwtCurrentExpire time.Time                   // Expiration time for the current JWT token
	jwtRefreshExpire time.Time                   // Expiration time for the refresh JWT token
	jwtAsyncRefresh  chan struct{}               // Channel tracking if an async refresher is running
	jwtRefresherStop chan chan struct{}          // Notification channel to stop the JWT refresher
	jwtRefreshHook   func(skip bool, async bool) // Testing hook to monitor when a refresh is triggered
}

// Dial connects to a remote Bluesky server and exchanges some basic information
// to ensure the connectivity works.
func Dial(ctx context.Context, server string) (*Client, error) {
	return DialWithClient(ctx, server, new(http.Client))
}

// DialWithClient connects to a remote Bluesky server using a user supplied HTTP
// client and exchanges some basic information to ensure the connectivity works.
func DialWithClient(ctx context.Context, server string, client *http.Client) (*Client, error) {
	// Create the XRPC client from the supplied HTTP one
	local := &xrpc.Client{
		Client: client,
		Host:   server,
	}
	// Do a sanity check with the server to ensure everything works. We don't
	// really care about the response as long as we get a meaningful one.
	if _, err := atproto.ServerDescribeServer(ctx, local); err != nil {
		return nil, err
	}
	return &Client{
		client: local,
	}, nil
}

// Login authenticates to the Bluesky server with the given handle and appkey.
//
// Note, authenticating with a live password instead of an application key will
// be detected and rejected. For your security, this library will refuse to use
// your master credentials.
func (c *Client) Login(ctx context.Context, handle string, appkey string) error {
	// Authenticate to the Bluesky server
	sess, err := atproto.ServerCreateSession(ctx, c.client, &atproto.ServerCreateSession_Input{
		Identifier: handle,
		Password:   appkey,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrLoginUnauthorized, err)
	}
	// Verify and reject master credentials, sorry, no bad security practices
	token, _, err := jwt.NewParser().ParseUnverified(sess.AccessJwt, jwt.MapClaims{})
	if err != nil {
		return err
	}
	if token.Claims.(jwt.MapClaims)["scope"] != "com.atproto.appPass" {
		return fmt.Errorf("%w: %w", ErrLoginUnauthorized, ErrMasterCredentials)
	}
	// Retrieve the expirations for the current and refresh JWT tokens
	current, err := token.Claims.GetExpirationTime()
	if err != nil {
		return err
	}
	if token, _, err = jwt.NewParser().ParseUnverified(sess.RefreshJwt, jwt.MapClaims{}); err != nil {
		return err
	}
	refresh, err := token.Claims.GetExpirationTime()
	if err != nil {
		return err
	}
	// Construct the authenticated client and the JWT expiration metadata
	c.client.Auth = &xrpc.AuthInfo{
		AccessJwt:  sess.AccessJwt,
		RefreshJwt: sess.RefreshJwt,
		Handle:     sess.Handle,
		Did:        sess.Did,
	}
	c.jwtCurrentExpire = current.Time
	c.jwtRefreshExpire = refresh.Time

	c.jwtAsyncRefresh = make(chan struct{}, 1) // 1 async refresher allowed concurrently
	c.jwtRefresherStop = make(chan chan struct{})
	go c.refresher()

	return nil
}

// Close terminates the client, shutting down all pending tasks and background
// operations.
func (c *Client) Close() error {
	// If the periodical JWT refresher is running, tear it down
	if c.jwtRefresherStop != nil {
		stopc := make(chan struct{})
		c.jwtRefresherStop <- stopc
		<-stopc

		c.jwtRefresherStop = nil
	}
	return nil
}

// refresher is an infinite loop that periodically checks the validity of the JWT
// tokens and runs a refresh cycle if they are getting close to expiration.
func (c *Client) refresher() {
	for {
		// Attempt to refresh the JWT token
		c.maybeRefreshJWT()

		// Wait until some time passes or the client is closing down
		select {
		case <-time.After(time.Minute):
		case stopc := <-c.jwtRefresherStop:
			stopc <- struct{}{}
			return
		}
	}
}

// maybeRefreshJWT checks the remainder validity time of the JWT token and does
// a session refresh if it is necessary. Depending on the amount of time it is
// still valid it might attempt a refresh on a background thread (permitting the
// current thread to proceed) or blocking the thread and doing a sync refresh.
func (c *Client) maybeRefreshJWT() error {
	// If the JWT token is still valid for a long time, use as is
	c.jwtLock.RLock()
	var (
		now        = time.Now()
		validAsync = c.jwtCurrentExpire.Sub(now) > jwtAsyncRefreshThreshold
		validSync  = c.jwtCurrentExpire.Sub(now) > jwtSyncRefreshThreshold
	)
	c.jwtLock.RUnlock()

	if validAsync {
		return nil
	}
	// If the JWT token is still valid enough for an async refresh, do that and
	// not block the API call for it
	if validSync {
		select {
		case c.jwtAsyncRefresh <- struct{}{}:
			// We're the first to attempt a background refresh, do it
			go func() {
				c.refreshJWT(true)
				<-c.jwtAsyncRefresh
			}()
			return nil

		default:
			// Someone else is already doing a background refresh, let them
			return nil
		}
	}
	// We've run out of the background refresh window, block the client on a
	// synchronous refresh
	c.jwtLock.Lock()
	defer c.jwtLock.Unlock()

	return c.refreshJWT(false)
}

// refreshJWT updates the JWT token and swaps out the credentials in the client.
//
// The async flag signals to the method whether it's running in async mode needing
// locking to access the JWT fields or if it was locked and can yolo it directly.
func (c *Client) refreshJWT(async bool) error {
	// Double-check the JWT token's validity to avoid multiple concurrent calls
	// being blocked and each refreshing the token. Async refresh is guaranteed
	// to be single threaded so no need to recheck the threshold with that.
	if !async && time.Until(c.jwtCurrentExpire) > jwtAsyncRefreshThreshold {
		// JWT token was already refreshed by someone else, ignore request
		if c.jwtRefreshHook != nil {
			c.jwtRefreshHook(true, async)
		}
		return nil
	}
	if c.jwtRefreshHook != nil {
		c.jwtRefreshHook(false, async)
	}
	// If the refresh token got invalidated too, bad luck
	var expires time.Time
	if async {
		c.jwtLock.RLock()
	}
	expires = c.jwtRefreshExpire
	if async {
		c.jwtLock.RUnlock()
	}
	if time.Until(expires) < 0 {
		return fmt.Errorf("%w: refresh token was valid until %v", ErrSessionExpired, expires)
	}
	// Attempt to refresh the JWT token. Since the client might be used async
	// for other requests, create a copy with the fields we need to mess with.
	refClient := new(xrpc.Client)
	if async {
		c.jwtLock.RLock()
	}
	*refClient = *c.client
	refClient.Auth = new(xrpc.AuthInfo)
	*refClient.Auth = *c.client.Auth

	refClient.Auth.AccessJwt = refClient.Auth.RefreshJwt
	if async {
		c.jwtLock.RUnlock()
	}
	sess, err := atproto.ServerRefreshSession(context.Background(), refClient)
	if err != nil {
		return err
	}
	// Update the JWT token in the local client
	token, _, err := jwt.NewParser().ParseUnverified(sess.AccessJwt, jwt.MapClaims{})
	if err != nil {
		return err
	}
	current, err := token.Claims.GetExpirationTime()
	if err != nil {
		return err
	}
	token, _, err = jwt.NewParser().ParseUnverified(sess.RefreshJwt, jwt.MapClaims{})
	if err != nil {
		return err
	}
	refresh, err := token.Claims.GetExpirationTime()
	if err != nil {
		return err
	}
	// Update the authenticated client and the JWT expiration metadata
	if async {
		c.jwtLock.Lock()
		defer c.jwtLock.Unlock()
	}
	c.client.Auth = &xrpc.AuthInfo{
		AccessJwt:  sess.AccessJwt,
		RefreshJwt: sess.RefreshJwt,
		Handle:     sess.Handle,
		Did:        sess.Did,
	}
	c.jwtCurrentExpire = current.Time
	c.jwtRefreshExpire = refresh.Time

	return nil
}

// CustomCall is a wildcard method for executing atproto API calls that are not
// (yet?) implemented by this library. The user needs to provide a callback that
// will receive an XRPC client to do direct atproto calls through.
//
// Note, the caller should not hold onto the xrpc.Client. The client is a copy
// of the internal one and will not receive JWT token updates, so it *will* be
// a dud after the JWT expiration time passes.
func (c *Client) CustomCall(callback func(client *xrpc.Client) error) error {
	// Refresh the JWT tokens before doing any user calls
	c.maybeRefreshJWT()

	// Create a copy of the xrpc client for power users
	dangling := new(xrpc.Client)

	c.jwtLock.RLock()
	*dangling = *c.client
	*dangling.Auth = *c.client.Auth

	if c.client.AdminToken != nil {
		dangling.AdminToken = new(string)
		*dangling.AdminToken = *c.client.AdminToken
	}
	if c.client.UserAgent != nil {
		dangling.UserAgent = new(string)
		*dangling.UserAgent = *c.client.UserAgent
	}
	c.jwtLock.RUnlock()

	// Run the user's callback against the copy of the authorized client
	return callback(dangling)
}
