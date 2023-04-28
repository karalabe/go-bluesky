# Bluesky API client from Go

This is a [Bluesky](https://bsky.app/) client library written in Go. It requires a Bluesky account
to connect through and an application password to authenticate with.

This library is ***highly opinionated*** and built around my personal preferences as to how Go code
should look and behave. My goals are simplicity and security rather than flexibility.

*Disclaimer: The state of the library is not even pre-alpha. Everything can change, everything can
blow up, nothing may work, the whole thing might get abandoned. Don't expect API stability.*

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