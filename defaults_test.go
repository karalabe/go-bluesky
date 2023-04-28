// Copyright 2023 Péter Szilágyi. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"os"
)

// These are the global authentication credentials to the Bluesky server to test the client library
var (
	testAuthHandle string
	testAuthPasswd string
	testAuthAppkey string
)

func init() {
	if testAuthHandle = os.Getenv("GOBLUESKY_TEST_HANDLE"); testAuthHandle == "" {
		panic("GOBLUESKY_TEST_HANDLE must be set to run the tests")
	}
	if testAuthPasswd = os.Getenv("GOBLUESKY_TEST_PASSWD"); testAuthPasswd == "" {
		panic("GOBLUESKY_TEST_PASSWD must be set to run the tests")
	}
	if testAuthAppkey = os.Getenv("GOBLUESKY_TEST_APPKEY"); testAuthAppkey == "" {
		panic("GOBLUESKY_TEST_APPKEY must be set to run the tests")
	}
}
