// Copyright 2023 go-bluesky authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bluesky

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"

	"github.com/bluesky-social/indigo/api/bsky"
)

const (
	// maxProfileAvatarBytes is the maximum number of bytes a profile avatar might
	// have before it's rejected by the library.
	maxProfileAvatarBytes = 8 * 1024 * 1024

	// maxProfileBannerBytes is the maximum number of bytes a profile banner might
	// have before it's rejected by the library.
	maxProfileBannerBytes = 8 * 1024 * 1024
)

// Profile represents a user profile on a Bluesky server.
type Profile struct {
	client *Client // Embedded API client to lazy-load pictures

	Handle string // User-friendly - unstable - identifier for the user
	DID    string // Machine friendly - stable - identifier for the user
	Name   string // Display name to use in various apps
	Bio    string // Profile description to use in various apps

	AvatarURL string      // CDN URL to the user's profile picture, empty if unset
	Avatar    image.Image // Profile picture, nil if unset or not yet resolved

	BannerURL string      // CDN URL to the user's banner picture, empty if unset
	Banner    image.Image // Banner picture, nil if unset ot not yet resolved

	FollowerCount uint    // Number of people who follow this user
	Followers     []*User // Actual list of followers, nil if not yet resolved
	FolloweeCount uint    // Number of people who this user follows
	Followees     []*User // Actual list of followees, nil if not yet resolved

	PostCount uint // Number of posts this user made
}

// User tracks some metadata about a user on a Bluesky server.
type User struct {
	client *Client // Embedded API client to lazy-load pictures

	Handle string // User-friendly - unstable - identifier for the follower
	DID    string // Machine friendly - stable - identifier for the follower
	Name   string // Display name to use in various apps
	Bio    string // Profile description to use in various apps

	AvatarURL string      // CDN URL to the user's profile picture, empty if unset
	Avatar    image.Image // Profile picture, nil if unset or not yet fetched
}

// FetchProfile retrieves all the metadata about a specific user.
//
// Supported IDs are the Bluesky handles or atproto DIDs.
func (c *Client) FetchProfile(ctx context.Context, id string) (*Profile, error) {
	// The API only supports the non-prefixed forms. Seems a bit wonky, but trim
	// manually for now until it's decided whether this is a feature or a bug.
	// https://github.com/bluesky-social/atproto/issues/989
	if strings.HasPrefix(id, "@") {
		id = id[1:]
	}
	if strings.HasPrefix(id, "at://") {
		id = id[5:]
	}
	// Retrieve the remote profile
	profile, err := bsky.ActorGetProfile(ctx, c.client, id)
	if err != nil {
		return nil, err
	}
	// Dig out the relevant fields and drop pointless pointers
	p := &Profile{
		client:        c,
		Handle:        profile.Handle,
		DID:           profile.Did,
		FollowerCount: uint(*profile.FollowersCount),
		FolloweeCount: uint(*profile.FollowsCount),
		PostCount:     uint(*profile.PostsCount),
	}
	if profile.DisplayName != nil {
		p.Name = *profile.DisplayName
	}
	if profile.Description != nil {
		p.Bio = *profile.Description
	}
	if profile.Avatar != nil {
		p.AvatarURL = *profile.Avatar
	}
	if profile.Banner != nil {
		p.BannerURL = *profile.Banner
	}
	return p, nil
}

// String implements the stringer interface to help debug things.
func (p *Profile) String() string {
	if p.Name == "" {
		return fmt.Sprintf("%s (%s)", p.Handle, p.DID)
	}
	return fmt.Sprintf("%s (%s/%s)", maybeEscape(p.Name), p.Handle, p.DID)
}

// ResolveAvatar resolves the profile avatar from the server URL and injects it
// into the profile itself. If the avatar (URL) is unset, the method will return
// success and leave the image in the profile nil.
//
// Note, the method will place a sanity limit on the maximum size of the image
// in bytes to avoid malicious content. You may use the ResolveAvatarWithLimit to
// override and potentially disable this protection.
func (p *Profile) ResolveAvatar(ctx context.Context) error {
	return p.ResolveAvatarWithLimit(ctx, maxProfileAvatarBytes)
}

// ResolveAvatarWithLimit resolves the profile avatar from the server URL using a
// custom data download limit (set to 0 to disable entirely) and injects it into
// the profile itself. If the avatar (URL) is unset, the method will return success
// and leave the image in the profile nil.
func (p *Profile) ResolveAvatarWithLimit(ctx context.Context, bytes uint64) error {
	if p.AvatarURL == "" {
		return nil
	}
	avatar, err := fetchImage(ctx, p.client, p.AvatarURL, bytes)
	if err != nil {
		return err
	}
	p.Avatar = avatar
	return nil
}

// ResolveBanner resolves the profile banner from the server URL and injects it
// into the profile itself. If the banner (URL) is unset, the method will return
// success and leave the image in the profile nil.
//
// Note, the method will place a sanity limit on the maximum size of the image
// in bytes to avoid malicious content. You may use the ResolveBannerWithLimit to
// override and potentially disable this protection.
func (p *Profile) ResolveBanner(ctx context.Context) error {
	return p.ResolveBannerWithLimit(ctx, maxProfileBannerBytes)
}

// ResolveBannerWithLimit resolves the profile banner from the server URL using a
// custom data download limit (set to 0 to disable entirely) and injects it into
// the profile itself. If the banner (URL) is unset, the method will return success
// and leave the image in the profile nil.
func (p *Profile) ResolveBannerWithLimit(ctx context.Context, bytes uint64) error {
	if p.BannerURL == "" {
		return nil
	}
	banner, err := fetchImage(ctx, p.client, p.BannerURL, bytes)
	if err != nil {
		return err
	}
	p.Banner = banner
	return nil
}

// ResolveFollowers resolves the full list of followers of a profile and injects
// it into the profile itself.
//
// Note, since there is a fairly low limit on retrievable followers per API call,
// this method might take a while to complete on larger accounts. You may use the
// ResolveFollowersStreaming to have finer control over the rate of retrievals,
// interruptions and memory usage.
func (p *Profile) ResolveFollowers(ctx context.Context) error {
	followerc, errc := p.ResolveFollowersStreaming(ctx)

	followers := make([]*User, 0, p.FollowerCount)
	for follower := range followerc {
		followers = append(followers, follower)
	}
	if err := <-errc; err != nil {
		return err
	}
	p.Followers = followers
	return nil
}

// ResolveFollowersStreaming gradually resolves the full list of followers of
// a profile, feeding them async into a result channel, closing the channel when
// there are no more followers left. An error channel is also returned and will
// receive (optionally, only ever one) error in case of a failure.
//
// Note, this method is meant to process the follower list as a stream, and will
// thus not populate the profile's followers field.
func (p *Profile) ResolveFollowersStreaming(ctx context.Context) (<-chan *User, <-chan error) {
	var (
		cursor    string
		followers = make(chan *User, 100) // Ensure all results fit to unblock a second call
		errc      = make(chan error, 1)   // Ensure the failure fits to unblock termination
	)
	go func() {
		// No matter what happens, close both channels
		defer func() {
			close(followers)
			close(errc)
		}()
		for {
			// Resolve the followers from the Bluesky server
			res, err := bsky.GraphGetFollowers(ctx, p.client.client, p.DID, cursor, 100)
			if err != nil {
				errc <- err
				return
			}
			// Parse the followers and feed them one by one to the sink channel
			for _, follower := range res.Followers {
				f := &User{
					client: p.client,
					Handle: follower.Handle,
					DID:    follower.Did,
				}
				if follower.DisplayName != nil {
					f.Name = *follower.DisplayName
				}
				if follower.Description != nil {
					f.Bio = *follower.Description
				}
				if follower.Avatar != nil {
					f.AvatarURL = *follower.Avatar
				}
				select {
				case <-ctx.Done():
					// Request is being torn down, abort
					errc <- ctx.Err()
					return
				case followers <- f:
					// Follower read, get the next one
				}
			}
			// If there are further followers to parse, repeat
			if res.Cursor == nil {
				break
			}
			cursor = *res.Cursor
		}
	}()
	return followers, errc
}

// ResolveFollowees resolves the full list of followees of a profile and injects
// it into the profile itself.
//
// Note, since there is a fairly low limit on retrievable followees per API call,
// this method might take a while to complete on larger accounts. You may use the
// ResolveFolloweesStreaming to have finer control over the rate of retrievals,
// interruptions and memory usage.
func (p *Profile) ResolveFollowees(ctx context.Context) error {
	followeec, errc := p.ResolveFolloweesStreaming(ctx)

	followees := make([]*User, 0, p.FolloweeCount)
	for followee := range followeec {
		followees = append(followees, followee)
	}
	if err := <-errc; err != nil {
		return err
	}
	p.Followees = followees
	return nil
}

// ResolveFolloweesStreaming gradually resolves the full list of followees of
// a profile, feeding them async into a result channel, closing the channel when
// there are no more followees left. An error channel is also returned and will
// receive (optionally, only ever one) error in case of a failure.
//
// Note, this method is meant to process the followeer list as a stream, and will
// thus not populate the profile's followees field.
func (p *Profile) ResolveFolloweesStreaming(ctx context.Context) (<-chan *User, <-chan error) {
	var (
		cursor    string
		followees = make(chan *User, 100) // Ensure all results fit to unblock a second call
		errc      = make(chan error, 1)   // Ensure the failure fits to unblock termination
	)
	go func() {
		// No matter what happens, close both channels
		defer func() {
			close(followees)
			close(errc)
		}()
		for {
			// Resolve the followees from the Bluesky server
			res, err := bsky.GraphGetFollows(ctx, p.client.client, p.DID, cursor, 100)
			if err != nil {
				errc <- err
				return
			}
			// Parse the followers and feed them one by one to the sink channel
			for _, followee := range res.Follows {
				f := &User{
					client: p.client,
					Handle: followee.Handle,
					DID:    followee.Did,
				}
				if followee.DisplayName != nil {
					f.Name = *followee.DisplayName
				}
				if followee.Description != nil {
					f.Bio = *followee.Description
				}
				if followee.Avatar != nil {
					f.AvatarURL = *followee.Avatar
				}
				select {
				case <-ctx.Done():
					// Request is being torn down, abort
					errc <- ctx.Err()
					return
				case followees <- f:
					// Followee read, get the next one
				}
			}
			// If there are further followees to parse, repeat
			if res.Cursor == nil {
				break
			}
			cursor = *res.Cursor
		}
	}()
	return followees, errc
}

// String implements the stringer interface to help debug things.
func (u *User) String() string {
	if u.Name == "" {
		return fmt.Sprintf("%s (%s)", u.Handle, u.DID)
	}
	return fmt.Sprintf("%s (%s/%s)", maybeEscape(u.Name), u.Handle, u.DID)
}

// ResolveAvatar resolves the user avatar from the server URL and injects it into
// the user itself. If the avatar (URL) is unset, the method will return success
// and leave the image in the user nil.
//
// Note, the method will place a sanity limit on the maximum size of the image
// in bytes to avoid malicious content. You may use the ResolveAvatarWithLimit to
// override and potentially disable this protection.
func (u *User) ResolveAvatar(ctx context.Context) error {
	return u.ResolveAvatarWithLimit(ctx, maxProfileAvatarBytes)
}

// ResolveAvatarWithLimit resolves the user avatar from the server URL using a
// custom data download limit (set to 0 to disable entirely) and injects it into
// the user itself. If the avatar (URL) is unset, the method will return success
// and leave the image in the user nil.
func (u *User) ResolveAvatarWithLimit(ctx context.Context, bytes uint64) error {
	if u.AvatarURL == "" {
		return nil
	}
	avatar, err := fetchImage(ctx, u.client, u.AvatarURL, bytes)
	if err != nil {
		return err
	}
	u.Avatar = avatar
	return nil
}

// fetchImage resolves a remote image via a URL and a set byte cap.
func fetchImage(ctx context.Context, client *Client, url string, bytes uint64) (image.Image, error) {
	// Initiate the remote image retrieval
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.client.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	// Read the image with a cap on the max data size if requested
	in := io.Reader(res.Body)
	if bytes != 0 {
		in = io.LimitReader(res.Body, int64(bytes))
	}
	img, _, err := image.Decode(in)
	return img, err
}
