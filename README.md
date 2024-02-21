[![Moov Banner Logo](https://user-images.githubusercontent.com/20115216/104214617-885b3c80-53ec-11eb-8ce0-9fc745fb5bfc.png)](https://github.com/moov-io)

<p align="center">
  <a href="https://slack.moov.io/">Community</a>
  ·
  <a href="https://moov.io/blog/">Blog</a>
  <br>
  <br>
</p>

[![GoDoc](https://godoc.org/github.com/moov-io/go-sftp?status.svg)](https://godoc.org/github.com/moov-io/go-sftp)
[![Build Status](https://github.com/moov-io/go-sftp/workflows/Go/badge.svg)](https://github.com/moov-io/go-sftp/actions)
[![Coverage Status](https://codecov.io/gh/moov-io/go-sftp/branch/master/graph/badge.svg)](https://codecov.io/gh/moov-io/go-sftp)
[![Go Report Card](https://goreportcard.com/badge/github.com/moov-io/go-sftp)](https://goreportcard.com/report/github.com/moov-io/go-sftp)
[![Repo Size](https://img.shields.io/github/languages/code-size/moov-io/go-sftp?label=project%20size)](https://github.com/moov-io/go-sftp)
[![Apache 2 License](https://img.shields.io/badge/license-Apache2-blue.svg)](https://raw.githubusercontent.com/moov-io/ach/master/LICENSE)
[![Slack Channel](https://slack.moov.io/badge.svg?bg=e01563&fgColor=fffff)](https://slack.moov.io/)
[![GitHub Stars](https://img.shields.io/github/stars/moov-io/go-sftp)](https://github.com/moov-io/go-sftp)
[![Twitter](https://img.shields.io/twitter/follow/moov?style=social)](https://twitter.com/moov?lang=en)

# moov-io/go-sftp

Moov's mission is to give developers an easy way to create and integrate bank processing into their own software products. Our open source projects are each focused on solving a single responsibility in financial services and designed around performance, scalability, and ease of use.

GoSFTP provides a simple SFTP [client interface](https://pkg.go.dev/github.com/moov-io/go-sftp#Client) for uploading, listing, and opening files on an SFTP server.

```go
type Client interface {
	Ping() error
	Close() error

	Open(path string) (*File, error)
	Reader(path string) (*File, error)

	Delete(path string) error
	UploadFile(path string, contents io.ReadCloser) error

	ListFiles(dir string) ([]string, error)
	Walk(dir string, fn fs.WalkDirFunc) error
}
```

The library also includes a [mock client implementation](https://pkg.go.dev/github.com/moov-io/go-sftp#MockClient) which uses a local filesystem temporary directory for testing.

## Project status

Moov GoSFTP is actively used in production environments. Please star the project if you are interested in its progress. Please let us know if you encounter any bugs/unclear documentation or have feature suggestions by opening up an issue or pull request. Thanks!

## Getting help

| channel                                                     | info                                                                                                                                    |
|-------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------|
| Twitter [@moov](https://twitter.com/moov)	                  | You can follow Moov.io's Twitter feed to get updates on our project(s). You can also tweet us questions or just share blogs or stories. |
| [GitHub Issue](https://github.com/moov-io/go-sftp/issues)   | If you are able to reproduce a problem please open a GitHub Issue under the specific project that caused the error.                     |
| [moov-io slack](https://slack.moov.io/)                     | Join our slack channel (`#infra`) to have an interactive discussion about the development of the project.                                          |

## Supported and tested platforms

- 64-bit Linux (Ubuntu, Debian), macOS, and Windows

## Contributing

Yes please! Please review our [Contributing guide](CONTRIBUTING.md) and [Code of Conduct](CODE_OF_CONDUCT.md) to get started!

This project uses [Go Modules](https://go.dev/blog/using-go-modules) and Go v1.18 or newer. See [Golang's install instructions](https://golang.org/doc/install) for help setting up Go. You can download the source code and we offer [tagged and released versions](https://github.com/moov-io/go-sftp/releases/latest) as well. We highly recommend you use a tagged release for production.

### Releasing

To make a release of go-sftp simply open a pull request with `CHANGELOG.md` and `version.go` updated with the next version number and details. You'll also need to push the tag (i.e. `git push origin v1.0.0`) to origin in order for CI to make the release.

### Testing

We maintain a comprehensive suite of unit tests and recommend table-driven testing when a particular function warrants several very similar test cases. After starting the services with Docker Compose run all tests with `go test ./...`. Current overall coverage can be found on [Codecov](https://app.codecov.io/gh/moov-io/go-sftp/).

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.
