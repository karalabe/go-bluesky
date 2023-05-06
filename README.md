# Bluesky API client from Go

[![API Reference](
https://camo.githubusercontent.com/915b7be44ada53c290eb157634330494ebe3e30a/68747470733a2f2f676f646f632e6f72672f6769746875622e636f6d2f676f6c616e672f6764646f3f7374617475732e737667
)](https://pkg.go.dev/github.com/karalabe/go-bluesky)
[![Go Report Card](https://goreportcard.com/badge/github.com/karalabe/go-bluesky)](https://goreportcard.com/report/github.com/karalabe/go-bluesky)

This is a [Bluesky](https://bsky.app/) client library written in Go. It requires a Bluesky account
to connect through and an application password to authenticate with.

This library is ***highly opinionated*** and built around my personal preferences as to how Go code
should look and behave. My goals are simplicity and security rather than flexibility.

*Disclaimer: The state of the library is not even pre-alpha. Everything can change, everything can
blow up, nothing may work, the whole thing might get abandoned. Don't expect API stability.*

## Authentication

In order to authenticate to the Bluesky server, you will need a login handle and an application
password. The handle might be an email address or a username recognized by the Bluesky server. The
password, however, must be an application key. For security reasons this we will reject credentials
that allow full access to your user.

```go
import "errors"
import "github.com/karalabe/go-bluesky"

var (
	blueskyHandle = "example.com"
	blueskyAppkey = "1234-5678-9abc-def0"
)

func main() {
	ctx := context.Background()

	client, err := bluesky.Dial(ctx, bluesky.ServerBskySocial)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	err = client.Login(ctx, blueskyHandle, blueskyAppkey)
	switch {
		case errors.Is(err, bluesky.ErrMasterCredentials):
			panic("You're not allowed to use your full-access credentials, please create an appkey")
		case errors.Is(err, bluesky.ErrLoginUnauthorized):
			panic("Username of application password seems incorrect, please double check")
		case err != nil:
			panic("Something else went wrong, please look at the returned error")
	}
}
```

Of course, most of the time you won't care about the errors broken down like that. Logging the error
and failing is probably enough in general, the introspection is meant for strange power uses.

The above code will create a client authenticated against the given Bluesky server. The client will
automatically refresh the authorization token internally when it closes in on expiration. The auth 
will be attempted to be refreshed async without blocking API calls if there's enough time left, or
by blocking if it would be cutting it too close to expiration (or already expired).

## Profiles and images

Any user's profile can be retrieved via the `bluesky.Client.FetchProfile` method. This will return
some basic metadata about the user.

```go
profile, err := client.FetchProfile(ctx, "karalabe.bsky.social")
if err != nil {
	panic(err)
}
fmt.Println("Name:", profile.Name)
fmt.Println("  - Followers:", profile.FollowerCount)
fmt.Println("  - Follows:", profile.FolloweeCount)
fmt.Println("  - Posts:", profile.PostCount)
```

Certain fields, like avatars and banner images are not retrieved by default since they are probably
large and most use cases don't need them. If the image URLs are not enough, the images themselves
can also be retrieved lazily into `image.Image` fields.

```go
if err := profile.ResolveAvatar(ctx); err != nil {
	panic(err)
}
fmt.Println("Avatar size:", profile.Avatar.Bounds())

if err := profile.ResolveBanner(ctx); err != nil {
	panic(err)
}
fmt.Println("Banner size:", profile.Banner.Bounds())
```

## Social graph

Being a social network, there's not much fun without being able to hop though the social graph. A
user profile is a good starting point to do that! The list of followers and followees of a user can
both be retrieved lazily after fetching the profile.

```go
fmt.Println("User followed by:")
if err := profile.ResolveFollowers(ctx); err != nil {
	panic(err)
}
for _, follower := range profile.Followers {
	fmt.Println("  -", follower)
}

fmt.Println("User follows:")
if err := profile.ResolveFollowees(ctx); err != nil {
    panic(err)
}
for _, followee := range profile.Followees {
	fmt.Println("  -", followee)
}
```

The above resolvers are elegant, self-contained methods, but if the follower/followee count of a
user is significant, it might be suboptimal to just wait hoping for the method to eventually return
without using up too much memory. A more powerful way is to request the follower/followees to be
returned as a stream as they are crawled!

```go
fmt.Println("User followed by:")
followerc, errc := profile.ResolveFollowersWithChannel(ctx)
for follower := range followerc { // Pull the users from the channel as they arrive
	fmt.Println("  -", follower)
}
if err := <-errc; err != nil {
	panic(err)
}

fmt.Println("User follows:")
followeec, errc := profile.ResolveFollowersWithChannel(ctx)
for followee := range followeec { // Pull the users from the channel as they arrive
	fmt.Println("  -", followee)
}
if err := <-errc; err != nil {
	panic(err)
}
```

Of course, as with the user profiles, follower and followee items also contain certain lazy resolvable
fields like the profile picture. In order however to crawl the social graph further, you will need to
fetch the profile of a follower/followee first and go from there.

## Custom API calls

As with any client library, there will inevitably come the time when the user wants to call something
that is not wrapped (or not yet implemented because it's a new server feature). For those power use
cases, the library exposes a custom caller that can be used to tap directly into the [atproto
APIs](https://pkg.go.dev/github.com/bluesky-social/indigo/api/atproto).

The custom caller will provide the user with an `xrpc.Client` that has valid user credentials and the
user can do arbitrary atproto calls with it.

```go
client.CustomCall(func(api *xrpc.Client) error {
	_, err := atproto.ServerGetSession(context.Background(), api)
	return err
})
```

*Note, the user should not retain the `xprc.Client` given to the callback as this is only a copy of
the internal one and will not be updated with new JWT tokens when the old ones are expired.*

## Testing

Oh boy, you're gonna freak out 😅. Since there's no Go implementation of a Bluesky API server and
PDS, there's nothing to run the test against... apart from the live system 😱.

To run the tests, you will have to provide authentication credentials to interact with the official
Bluesky server. Needless to say, your testing account may not become the most popular with all the
potential spam it might generate, so be prepared to lose it. ¯\_(ツ)_/¯

To run the tests, set the `GOBLUESKY_TEST_HANDLE`, `GOBLUESKY_TEST_PASSWD` and `GOBLUESKY_TEST_APPKEY`
env vars and run the tests via the normal Go workflow.

```sh
$ export GOBLUESKY_TEST_HANDLE=example.com
$ export GOBLUESKY_TEST_PASSWD=my-pass-phrase
$ export GOBLUESKY_TEST_APPKEY=1234-5678-9abc-def0

$ go test -v
```

## License

3-Clause BSD