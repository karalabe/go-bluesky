// Copyright 2023 Péter Szilágyi. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"errors"
	"testing"
	"time"
)

// Basic test to see if connecting to a Bluesky instance works.
func TestDial(t *testing.T) {
	if _, err := Dial(ServerBskySocial); err != nil {
		t.Fatalf("failed to dial bluesky server: %v", err)
	}
}

// Tests that logging into a Bluesky server works and also that only app passwords
// are accepted, rejecting master credentials.
func TestLogin(t *testing.T) {
	client, _ := Dial(ServerBskySocial)

	if err := client.Login(testAuthHandle, "definitely-not-my-password"); !errors.Is(err, ErrLoginUnauthorized) {
		t.Errorf("invalid password error mismatch: have %v, want %v", err, ErrLoginUnauthorized)
	}
	if err := client.Login(testAuthHandle, testAuthPasswd); !errors.Is(err, ErrLoginUnauthorized) || !errors.Is(err, ErrMasterCredentials) {
		t.Errorf("master password error mismatch: have %v, want %v: %v", err, ErrLoginUnauthorized, ErrMasterCredentials)
	}
	if err := client.Login(testAuthHandle, testAuthAppkey); err != nil {
		t.Errorf("app password error mismatch: have %v, want %v", err, nil)
	}
}

// Tests that the JWT token will not get refreshed if it's still valid.
func TestJWTNoopRefresh(t *testing.T) {
	client, _ := Dial(ServerBskySocial)
	client.Login(testAuthHandle, testAuthAppkey)

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
	client, _ := Dial(ServerBskySocial)
	client.Login(testAuthHandle, testAuthAppkey)

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
	client, _ := Dial(ServerBskySocial)
	client.Login(testAuthHandle, testAuthAppkey)

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
	client, _ := Dial(ServerBskySocial)
	client.Login(testAuthHandle, testAuthAppkey)

	client.jwtCurrentExpire = time.Time{}
	client.jwtRefreshExpire = time.Time{}

	if err := client.maybeRefreshJWT(); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expired session error mismatch: have %v, want %v", err, ErrSessionExpired)
	}
}
