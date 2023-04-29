# Bluesky API client from Go

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
import "github.com/karalabe/go-bluesky"

var (
	blueskyHandle = "example.com"
	blueskyAppkey = "1234-5678-9abc-def0"
)

func main() {
	client, err := Dial(bluesky.ServerBskySocial)
	if err != nil {
		panic(err)
	}
	err := client.Login(blueskyHandle, blueskyAppkey)
	select {
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