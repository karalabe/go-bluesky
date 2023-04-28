// Copyright 2023 Péter Szilágyi. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"context"
	"errors"
	"fmt"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrLoginUnauthorized is returned from a login attempt if the credentials
	// are rejected by the server or the local client (master credentials).
	ErrLoginUnauthorized = errors.New("unauthorized")

	// ErrMasterCredentials is returned from a login attempt if the credentials
	// are valid on the Bluesky server, but they are the user's master password.
	// Since that is a security malpractice, this library forbids it.
	ErrMasterCredentials = errors.New("master credentials used")
)

// Client is an API client attached to (and authenticated to) a Bluesky PDS instance.
type Client struct {
	client *xrpc.Client
}

// Dial connects to a remote Bluesky server and exchanges some basic information
// to ensure the connectivity works.
func Dial(server string) (*Client, error) {
	return DialWithClient(server, new(xrpc.Client))
}

// DialWithClient connects to a remote Bluesky server using a user supplied HTTP
// (XRPC) client and exchanges some basic information to ensure the connectivity
// works.
//
// Note, the host configuration of the provided client is ignored and rather the
// one specified as the dial server will be used.
func DialWithClient(server string, client *xrpc.Client) (*Client, error) {
	// Copy the client to override the host field
	local := new(xrpc.Client)
	*local = *client
	local.Host = server

	// Do a sanity check with the server to ensure everything works. We don't
	// really care about the response as long as we get a meaningful one.
	if _, err := atproto.ServerDescribeServer(context.Background(), local); err != nil {
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
func (c *Client) Login(handle string, appkey string) error {
	// Authenticate to the Bluesky server
	sess, err := atproto.ServerCreateSession(context.Background(), c.client, &atproto.ServerCreateSession_Input{
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
	c.client.Auth = &xrpc.AuthInfo{
		AccessJwt:  sess.AccessJwt,
		RefreshJwt: sess.RefreshJwt,
		Handle:     sess.Handle,
		Did:        sess.Did,
	}
	return nil
}
