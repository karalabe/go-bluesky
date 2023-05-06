// Copyright 2023 go-bluesky authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/xrpc"
)

// testCredentials contains credentials from the environment to use for Client
// login tests.
type testCredentials struct {
	handle string
	passwd string
	appkey string
}

// makeTestClient returns a Client and authentication credentials from env vars
// that can be used to log in. The test will be skipped if the required variables
// are not set.
func makeTestClient(t *testing.T) (*Client, *testCredentials) {
	t.Helper()

	var (
		handle = getenvOrSkip(t, "GOBLUESKY_TEST_HANDLE")
		passwd = getenvOrSkip(t, "GOBLUESKY_TEST_PASSWD")
		appkey = getenvOrSkip(t, "GOBLUESKY_TEST_APPKEY")
	)
	client, err := Dial(context.Background(), ServerBskySocial)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	return client, &testCredentials{
		handle: handle,
		passwd: passwd,
		appkey: appkey,
	}
}

// makeTestClientWithLogin returns a Client which is logged in using credentials
// from the environment. The test will be skipped if the required env vars are
// not set.
func makeTestClientWithLogin(t *testing.T) *Client {
	t.Helper()

	client, creds := makeTestClient(t)
	if err := client.Login(context.Background(), creds.handle, creds.appkey); err != nil {
		t.Fatalf("failed to login to Bluesky server: %v", err)
	}
	return client
}

// getenvOrSkip fetches the value of env or skips the test if env is not set.
func getenvOrSkip(t *testing.T, env string) string {
	t.Helper()

	val, ok := os.LookupEnv(env)
	if !ok {
		t.Skipf("skipping, environment variable %q is required to run the tests", env)
	}
	return val
}

// Basic test to see if connecting to a Bluesky instance works.
func TestDial(t *testing.T) {
	ctx := context.Background()
	client, err := Dial(ctx, ServerBskySocial)
	if err != nil {
		t.Fatalf("failed to dial bluesky server: %v", err)
	}
	defer client.Close()
}

// Tests that logging into a Bluesky server works and also that only app passwords
// are accepted, rejecting master credentials.
func TestLogin(t *testing.T) {
	client, creds := makeTestClient(t)
	ctx := context.Background()

	if err := client.Login(ctx, creds.handle, "definitely-not-my-password"); !errors.Is(err, ErrLoginUnauthorized) {
		t.Errorf("invalid password error mismatch: have %v, want %v", err, ErrLoginUnauthorized)
	}
	if err := client.Login(ctx, creds.handle, creds.passwd); !errors.Is(err, ErrLoginUnauthorized) || !errors.Is(err, ErrMasterCredentials) {
		t.Errorf("master password error mismatch: have %v, want %v: %v", err, ErrLoginUnauthorized, ErrMasterCredentials)
	}
	if err := client.Login(ctx, creds.handle, creds.appkey); err != nil {
		t.Errorf("app password error mismatch: have %v, want %v", err, nil)
	}
}

// Tests that the JWT token will not get refreshed if it's still valid.
func TestJWTNoopRefresh(t *testing.T) {
	client := makeTestClientWithLogin(t)

	errc := make(chan error, 1)
	client.jwtRefreshHook = func(skip bool, async bool) {
		errc <- errors.New("jwt token refresher ran while original was valid")
	}
	client.maybeRefreshJWT()

	select {
	case err := <-errc:
		t.Fatal(err.Error())
	case <-time.After(100 * time.Millisecond):
		// refresher didn't run, everything as expected
	}
}

// Tests that the JWT token can be refreshed async if the expiration time becomes
// less than the allowed window.
func TestJWTAsyncRefresh(t *testing.T) {
	client := makeTestClientWithLogin(t)

	errc := make(chan error, 1)
	client.jwtRefreshHook = func(skip bool, async bool) {
		if skip {
			errc <- errors.New("jwt refresher skipped refresh below async validity threshold")
			return
		}
		if !async {
			errc <- errors.New("jwt refresher attempted blocking refresh above sync validity threshold")
			return
		}
		errc <- nil
	}
	client.jwtCurrentExpire = time.Now().Add(jwtAsyncRefreshThreshold - time.Second)
	client.maybeRefreshJWT()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatal(err.Error())
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("jwt token refreshed didn't get called")
	}
	// Wait a bit for background refresh (ush) and check that the JWT token was refreshed
	time.Sleep(500 * time.Millisecond)
	if time.Until(client.jwtCurrentExpire) < jwtAsyncRefreshThreshold {
		t.Fatalf("jwt token refresh failed")
	}
}

// Tests that the JWT token will be refreshed sync if the expiration time becomes
// less than the allowed window.
func TestJWTSyncRefresh(t *testing.T) {
	client := makeTestClientWithLogin(t)

	errc := make(chan error, 1)
	client.jwtRefreshHook = func(skip bool, async bool) {
		if skip {
			errc <- errors.New("jwt refresher skipped refresh below sync validity threshold")
			return
		}
		if async {
			errc <- errors.New("jwt refresher attempted async refresh below sync validity threshold")
			return
		}
		errc <- nil
	}
	client.jwtCurrentExpire = time.Now().Add(jwtSyncRefreshThreshold - time.Second)
	client.maybeRefreshJWT()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatal(err.Error())
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("jwt token refreshed didn't get called")
	}
	// Check immediately that the JWT token was refreshed
	if time.Until(client.jwtCurrentExpire) < jwtAsyncRefreshThreshold {
		t.Fatalf("jwt token refresh failed")
	}
}

// Tests that if even the JWT refresh token got expired, teh refresher errors
// out synchronously.
func TestJWTExpiredRefresh(t *testing.T) {
	client := makeTestClientWithLogin(t)
	client.jwtCurrentExpire = time.Time{}
	client.jwtRefreshExpire = time.Time{}

	if err := client.maybeRefreshJWT(); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expired session error mismatch: have %v, want %v", err, ErrSessionExpired)
	}
}

// Tests that the library can be used to do custom atproto calls directly if some
// operation is not implemented.
func TestCustomCall(t *testing.T) {
	client := makeTestClientWithLogin(t)
	err := client.CustomCall(func(api *xrpc.Client) error {
		_, err := atproto.ServerGetSession(context.Background(), api)
		return err
	})
	if err != nil {
		t.Fatalf("failed to execute custom call: %v", err)
	}
}
