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
[![Go Report Card](https://goreportcard.com/badge/github.com/moov-io/go-sftp)](https://goreportcard.com/report/github.com/moov-io/go-sftp)
[![Apache 2 License](https://img.shields.io/badge/license-Apache2-blue.svg)](https://raw.githubusercontent.com/moov-io/go-sftp/master/LICENSE)
[![Slack Channel](https://img.shields.io/badge/Slack-Moov-e01563)](https://slack.moov.io/)
[![Twitter](https://img.shields.io/twitter/follow/moov?style=social)](https://twitter.com/moov?lang=en)

# moov-io/go-sftp

Moov's mission is to give developers an easy way to create and integrate bank processing into their own software products. Our open source projects are each focused on solving a single responsibility in financial services and designed around performance, scalability, and ease of use.

moov-io/go-sftp provides a simple [SFTP client](https://pkg.go.dev/github.com/moov-io/go-sftp#Client) for uploading, listing, and opening files on an SFTP server.

This project requires **Go 1.26 or higher**. The current `go.mod` also sets `toolchain go1.27.4`.

The package name is `go_sftp`, so import it with an alias:

```go
import (
    "io"
    "strings"

    "github.com/moov-io/base/log"
    gosftp "github.com/moov-io/go-sftp"
)

func main() {
    client, err := gosftp.NewClient(log.NewDefaultLogger(), &gosftp.ClientConfig{
        Hostname:       "sftp.example.com:22",
        Username:       "alice",
        Password:       "correct-horse",
        HostPublicKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleHostKey"},
    })
    if err != nil {
        // client may still be non-nil; later calls will retry the connection
        panic(err)
    }
    defer client.Close()

    files, err := client.ListFiles("outbox")
    if err != nil {
        panic(err)
    }

    for _, path := range files {
        file, err := client.Reader(path)
        if err != nil {
            panic(err)
        }
        // ... read file.Contents, then file.Close()
        file.Close()
    }

    err = client.UploadFile("inbox/ppd.ach", io.NopCloser(strings.NewReader("file contents")))
    if err != nil {
        panic(err)
    }

    err = client.Delete("outbox/processed.ach")
    if err != nil {
        panic(err)
    }
}
```

A nil logger is allowed. `MaxConnections` defaults to 1 when it is left at zero.

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

`ListFiles` returns the files in `dir` (not subdirectories; use `Walk` for a recursive listing). It calls `ReadDir` first so relative paths work on servers that are not chrooted to `/`. After a successful `ReadDir` (including an empty directory), file names are joined onto the caller's dir. A directory component is rewritten only for an exact server match or a unique case-insensitive match. `ListFiles("/")` lists the working directory when `Getwd` is not `/`. Listing a symlink to a directory follows the symlink. If `ReadDir` fails after a live connection, ListFiles falls back to a case-insensitive glob; a lost connection is returned as an error. A named relative directory is joined with `Getwd` only when that walk is empty and the working directory is not `/`, and those join errors are returned. Returned paths use the same style as `dir` (relative or absolute).

`Delete` is idempotent: it returns nil if the file does not exist.

The library also includes a [mock client implementation](https://pkg.go.dev/github.com/moov-io/go-sftp#MockClient) which uses a local filesystem temporary directory for testing. `NewMockClient` requires a `*testing.T` and is intended for tests. The mock matches the real client for relative `ListFiles` paths, `Open`/`Reader` filenames, `ModTime`, and idempotent `Delete`.

## Security / host keys

If `HostPublicKey` and `HostPublicKeys` are both empty, the client skips host-key verification (`ssh.InsecureIgnoreHostKey`). A warning is logged (or written to standard error if the logger is nil). For any server that moves ACH or other financial files, pin the host key.

`HostPublicKeys` accepts one key per slice element, in either of these formats:

- an `authorized_keys` line, which is what `ssh-keyscan` prints
- a base64-encoded key

```bash
ssh-keyscan -t ed25519,rsa sftp.example.com
```

```go
cfg := &gosftp.ClientConfig{
    Hostname: "sftp.example.com:22",
    Username: "alice",
    Password: "correct-horse",
    HostPublicKeys: []string{
        "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleHostKey",
    },
}
```

The deprecated `HostPublicKey` field is still honored and merged into `HostPublicKeys`.

## Project status

moov-io/go-sftp is actively used in production environments. Please star the project if you are interested in its progress. Please let us know if you encounter any bugs/unclear documentation or have feature suggestions by opening up an issue or pull request. Thanks!

## Getting help

| channel                                                     | info                                                                                                                                    |
|-------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------|
| Twitter [@moov](https://twitter.com/moov)	                  | You can follow Moov.io's Twitter feed to get updates on our project(s). You can also tweet us questions or just share blogs or stories. |
| [GitHub Issue](https://github.com/moov-io/go-sftp/issues)   | If you are able to reproduce a problem please open a GitHub Issue under the specific project that caused the error.                     |
| [moov-io slack](https://slack.moov.io/)                     | Join our slack channel (`#infra`) to have an interactive discussion about the development of the project.                                          |

## Supported and tested platforms

- 64-bit Linux (Ubuntu, Debian), macOS, and Windows

## Contributing

Yes please! Please review our [Contributing guide](CONTRIBUTING.md) and [Code of Conduct](https://github.com/moov-io/moov-slack-code-of-conduct) to get started!

See [Golang's install instructions](https://go.dev/doc/install) for help setting up Go. You can download the source code and we offer [tagged and released versions](https://github.com/moov-io/go-sftp/releases/latest) as well. We highly recommend you use a tagged release for production.

### Releasing

To make a release of go-sftp simply open a pull request with `CHANGELOG.md` updated with the next version number and details. You'll also need to push the tag (i.e. `git push origin v1.0.0`) to origin in order for CI to make the release.

### Testing

Unit tests and examples can be run without Docker:

```
go test -short ./...
```

The integration tests in `client_test.go` and `network_test.go` talk to an SFTP server on `localhost:2222`. Docker and Docker Compose are required for that path:

```
make setup    # starts atmoz/sftp via docker compose
go test ./...
make teardown
```

`make test` runs `go test` with coverage. `make check` downloads the Moov lint script and needs network access.

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.
