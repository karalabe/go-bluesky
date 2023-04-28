// Copyright 2023 Péter Szilágyi. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"errors"
	"testing"
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
