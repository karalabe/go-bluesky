// Copyright 2023 go-bluesky authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"context"
	"errors"
	"io"
	"testing"
)

// Tests that the library can be used to fetch a user's profile from a Bluesky
// server.
func TestFetchProfileWithTrimmedHandle(t *testing.T) {
	testFetchProfile(t, testHandleTester)
}
func TestFetchProfileWithShorthandHandle(t *testing.T) {
	testFetchProfile(t, "@"+testHandleTester)
}
func TestFetchProfileWithCanonicalHandle(t *testing.T) {
	testFetchProfile(t, "at://"+testHandleTester)
}
func TestFetchProfileWithTrimmedDID(t *testing.T) {
	testFetchProfile(t, testDIDTester)
}
func TestFetchProfileWithCanonicalDID(t *testing.T) {
	testFetchProfile(t, "at://"+testDIDTester)
}

func testFetchProfile(t *testing.T, id string) {
	var (
		client = makeTestClientWithLogin(t)
		ctx    = context.Background()
	)
	// Retrieve and validate the base fields of the profile
	profile, err := client.FetchProfile(ctx, id)
	if err != nil {
		t.Fatalf("failed to fetch user profile: %v", err)
	}
	if profile.Handle != testHandleTester {
		t.Errorf("handle mismatch: have %v, want %v", profile.Handle, testHandleTester)
	}
	if profile.DID != testDIDTester {
		t.Errorf("did mismatch: have %v, want %v", profile.DID, testDIDTester)
	}
	if profile.Name != "go-bluesky tester" {
		t.Errorf("name mismatch: have %v, want %v", profile.Name, "go-bluesky tester")
	}
	if profile.Bio != "I'm a test account used to run the https://github.com/karalabe/go-bluesky test suite. If I do anything weird, please contact @karalabe.bsky.social to fix me." {
		t.Errorf("bio mismatch: have %v, want %v", profile.Bio, "I'm a test account used to run the https://github.com/karalabe/go-bluesky test suite. If I do anything weird, please contact @karalabe.bsky.social to fix me.")
	}
	if profile.AvatarURL != "" {
		t.Errorf("avatar URL mismatch: have %v, want %v", profile.AvatarURL, "")
	}
	if profile.BannerURL != "" {
		t.Errorf("banner URL mismatch: have %v, want %v", profile.BannerURL, "")
	}
	if profile.FollowerCount < 32 { // 06.05.2023 follower count
		t.Errorf("followers count mismatch: have %v, want at least %v", profile.FollowerCount, 32)
	}
	if profile.FolloweeCount != 1 { // only follow this lib's author
		t.Errorf("follows count mismatch: have %v, want %v", profile.FolloweeCount, 1)
	}
	if profile.FolloweeCount != 1 { // only follow this lib's author
		t.Errorf("follows count mismatch: have %v, want %v", profile.FolloweeCount, 1)
	}
	if profile.PostCount != 2 { // outside of live tests, there are only 2 stable posts on the user
		t.Errorf("posts count mismatch: have %v, want %v", profile.PostCount, 2)
	}
	// Avatar and banner resolution should succeed and be noops for the test user
	if err := profile.ResolveAvatar(ctx); err != nil {
		t.Errorf("avatar resolution failed for no avatar: %v", err)
	}
	if err := profile.ResolveBanner(ctx); err != nil {
		t.Errorf("banner resolution failed for no banner: %v", err)
	}
}

// Tests that the library can retrieve profile avatars and banners advertised.
func TestResolveProfileImages(t *testing.T) {
	var (
		client = makeTestClientWithLogin(t)
		ctx    = context.Background()
	)
	// Retrieve the library author's profile, hopefully the images are still set
	profile, err := client.FetchProfile(ctx, testDIDPeter)
	if err != nil {
		t.Fatalf("failed to fetch author profile: %v", err)
	}
	if profile.AvatarURL == "" {
		t.Errorf("avatar URL mismatch: have empty, want non-empty")
	}
	if profile.BannerURL == "" {
		t.Errorf("banner URL mismatch: have empty, want non-empty")
	}
	// Avatar and banner resolution should succeed and actually download something
	if err := profile.ResolveAvatar(ctx); err != nil {
		t.Errorf("avatar resolution failed: %v", err)
	}
	if profile.Avatar == nil {
		t.Errorf("resolved avatar still nil")
	}
	if err := profile.ResolveBanner(ctx); err != nil {
		t.Errorf("banner resolution failed: %v", err)
	}
	if profile.Banner == nil {
		t.Errorf("resolved banner still nil")
	}
	// Avatar and banner resolution should however fail if the user's desired
	// download limit is smaller than the images
	if err := profile.ResolveAvatarWithLimit(ctx, 100); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("avatar resolution error mismatch: have %v, want %v", err, io.ErrUnexpectedEOF)
	}
	if err := profile.ResolveBannerWithLimit(ctx, 100); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("banner resolution error mismatch: have %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

// Tests that the library can crawl the follower list and retrieve all of them.
func TestResolveProfileFollowers(t *testing.T) {
	var (
		client = makeTestClientWithLogin(t)
		ctx    = context.Background()
	)
	// Retrieve the library author's profile, hopefully there are many followers :P
	profile, err := client.FetchProfile(ctx, testDIDPeter)
	if err != nil {
		t.Fatalf("failed to fetch author profile: %v", err)
	}
	// Resolve all the followers directly into the profile struct
	if err := profile.ResolveFollowers(ctx); err != nil {
		t.Fatalf("failed to fetch author followers: %v", err)
	}
	if profile.Followers == nil {
		t.Errorf("embedded follower list nil")
	}
	if len(profile.Followers) != int(profile.FollowerCount) {
		t.Errorf("follower count mismatch: have %v, want %v", len(profile.Followers), profile.FollowerCount)
	}
}

// Tests that a cancelled context will stop resolving followers.
func TestResolveProfileFollowersWithCancellation(t *testing.T) {
	var (
		client = makeTestClientWithLogin(t)
		ctx    = context.Background()
	)
	// Retrieve the library author's profile, hopefully there are many followers :P
	profile, err := client.FetchProfile(ctx, testDIDPeter)
	if err != nil {
		t.Fatalf("failed to fetch author profile: %v", err)
	}
	// Resolve the followers indirectly via channels, cancelling after the first
	// read, ensuring that the full list does not get crawled
	cctx, cancel := context.WithCancel(ctx)
	followerc, errc := profile.ResolveFollowersWithChannel(cctx)

	<-followerc
	retrieved := 1

	cancel()
	for range followerc {
		retrieved++
	}
	if retrieved >= int(profile.FollowerCount) {
		t.Errorf("interrupted resolver retrieved all followers: have %d, want < %d", retrieved, profile.FollowerCount)
	}
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Errorf("interrupt error mismatch: have %v, want %v", err, context.Canceled)
	}
}

// Tests that avatars of followers can be resolved.
func TestResolveProfileFollowerAvatar(t *testing.T) {
	var (
		client = makeTestClientWithLogin(t)
		ctx    = context.Background()
	)
	// Retrieve the library author's profile and list of followers
	profile, err := client.FetchProfile(ctx, testDIDPeter)
	if err != nil {
		t.Fatalf("failed to fetch author profile: %v", err)
	}
	if err := profile.ResolveFollowers(ctx); err != nil {
		t.Fatalf("failed to fetch author followers: %v", err)
	}
	// Find Jeromy and hope he has a profile picture set, resolve it
	for _, follower := range profile.Followers {
		if follower.DID == testDIDJeromy {
			if err := follower.ResolveAvatar(ctx); err != nil {
				t.Errorf("failed to resolve follower avatar: %v", err)
			}
			if follower.Avatar == nil {
				t.Errorf("resolved avatar still nil")
			}
			break
		}
	}
}

// Tests that the library can crawl the followee list and retrieve all of them.
func TestResolveProfileFollowees(t *testing.T) {
	var (
		client = makeTestClientWithLogin(t)
		ctx    = context.Background()
	)
	// Retrieve the library author's profile, hopefully there are many followees :P
	profile, err := client.FetchProfile(ctx, testDIDPeter)
	if err != nil {
		t.Fatalf("failed to fetch author profile: %v", err)
	}
	// Resolve all the followees directly into the profile struct
	if err := profile.ResolveFollowees(ctx); err != nil {
		t.Fatalf("failed to fetch author followees: %v", err)
	}
	if profile.Followees == nil {
		t.Errorf("embedded followee list nil")
	}
	if len(profile.Followees) != int(profile.FolloweeCount) {
		t.Errorf("followee count mismatch: have %v, want %v", len(profile.Followees), profile.FolloweeCount)
	}
}

// Tests that a cancelled context will stop resolving followees.
func TestResolveProfileFolloweesWithCancellation(t *testing.T) {
	var (
		client = makeTestClientWithLogin(t)
		ctx    = context.Background()
	)
	// Retrieve Jeromy's profile, he's collecting followees like there's no tomorrow
	profile, err := client.FetchProfile(ctx, testDIDJeromy)
	if err != nil {
		t.Fatalf("failed to fetch author profile: %v", err)
	}
	// Resolve the followees indirectly via channels, cancelling after the first
	// read, ensuring that the full list does not get crawled
	cctx, cancel := context.WithCancel(ctx)
	followeec, errc := profile.ResolveFolloweesWithChannel(cctx)

	<-followeec
	retrieved := 1

	cancel()
	for range followeec {
		retrieved++
	}
	if retrieved >= int(profile.FolloweeCount) {
		t.Errorf("interrupted resolver retrieved all followees: have %d, want < %d", retrieved, profile.FolloweeCount)
	}
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Errorf("interrupt error mismatch: have %v, want %v", err, context.Canceled)
	}
}

// Tests that avatars of followees can be resolved.
func TestResolveProfileFolloweeAvatar(t *testing.T) {
	var (
		client = makeTestClientWithLogin(t)
		ctx    = context.Background()
	)
	// Retrieve the library author's profile and list of followees
	profile, err := client.FetchProfile(ctx, testDIDPeter)
	if err != nil {
		t.Fatalf("failed to fetch author profile: %v", err)
	}
	if err := profile.ResolveFollowees(ctx); err != nil {
		t.Fatalf("failed to fetch author followers: %v", err)
	}
	// Find Jeromy and hope he has a profile picture set, resolve it
	for _, followee := range profile.Followees {
		if followee.DID == testDIDJeromy {
			if err := followee.ResolveAvatar(ctx); err != nil {
				t.Errorf("failed to resolve followee avatar: %v", err)
			}
			if followee.Avatar == nil {
				t.Errorf("resolved avatar still nil")
			}
			break
		}
	}
}
