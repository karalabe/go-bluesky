// Copyright 2023 go-bluesky authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import "strconv"

// maybeEscape checks if the provided string needs escaping/quoting, and calls
// strconv.Quote if needed. The goal is to prevent malicious user input from
// potentially hijacking the user console.
func maybeEscape(s string) string {
	quote := false
	for _, r := range s {
		// We quote everything below <space> (0x20) and above~ (0x7E)
		if r < ' ' || r > '~' {
			quote = true
			break
		}
	}
	if !quote {
		return s
	}
	return strconv.Quote(s)
}
