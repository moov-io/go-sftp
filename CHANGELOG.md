## v0.13.0 (Released 2023-09-13)

IMPROVEMENTS

- feat: detect connection errors and force a reconnect
- meta: stop writing warning to stdout

BUILD

- fix(deps): update module golang.org/x/crypto to v0.13.0

## v0.12.2 (Released 2023-08-30)

IMPROVEMENTS

- fix: use the connection() helper to check for network errors

## v0.12.1 (Released 2023-08-28)

IMPROVEMENTS

- fix: check walker error before stat calls
- fix: handle special case for listfiles

BUILD

- fix(deps): update github.com/protonmail/go-crypto digest to 3c4c8a2
- fix(deps): update module github.com/go-kit/kit to v0.13.0

## v0.12.0 (Released 2023-08-23)

IMPROVEMENTS

- fix(deps): update module github.com/moov-io/base to v0.46.0
- fix: clarify ListFiles returns case intensive matches but returns server case
- test: add "list and read" checks

## v0.11.1 (Released 2023-08-18)

IMPROVEMENTS

- fix: open files for writing when writing

## v0.11.0 (Released 2023-08-17)

IMPROVEMENTS

- feat: add an option to skip directory creation on upload
- fix: sync files after writing to ensure storage, return .Close() error

## v0.10.3 (Released 2023-08-15)

IMPROVEMENTS

- test: verify full path is returned, Walk skips directories

## v0.10.2 (Released 2023-08-14)

BUILD

- fix(deps): update module github.com/pkg/sftp to v1.13.6

## v0.10.1 (Released 2023-08-08)

BUILD

- build: use the latest stable Go release
- build: update golang.org/x/crypto to v0.12.0
- fix(deps): update github.com/protonmail/go-crypto digest to 5aa5874
- fix(deps): update module github.com/moov-io/base to v0.45.1

## v0.10.0 (Released 2023-06-15)

IMPROVEMENTS

- feat: make File comply with the fs.File interface

BUILD

- build(deps): bump github.com/cloudflare/circl from 1.1.0 to 1.3.3
- fix(deps): update module github.com/moov-io/base to v0.44.0
- fix(deps): update module github.com/prometheus/client_golang to v1.16.0
- fix(deps): update module github.com/stretchr/testify to v1.8.4
- fix(deps): update module golang.org/x/crypto to v0.10.0

## v0.9.1 (Released 2023-03-17)

IMPROVEMENTS

- feat: wrap errors with `%w` so callers can unwrap them

BUILD

- fix(deps): update github.com/protonmail/go-crypto digest to cb82d93
- fix(deps): update module github.com/stretchr/testify to v1.8.2
- fix(deps): update module golang.org/x/crypto to v0.7.0

## v0.9.0 (Released 2023-02-01)

ADDITIONS

- feat: add Reader() method to clients for streaming contents
- feat: add Walk function for traversing a directory

BUILD

- fix(deps): update github.com/protonmail/go-crypto digest to d1d05f4
- fix(deps): update module github.com/moov-io/base to v0.39.0
- fix(deps): update module github.com/prometheus/client_golang to v1.14.0
- fix(deps): update module github.com/stretchr/testify to v1.8.1
- fix(deps): update module golang.org/x/crypto to v0.5.0

## v0.8.6 (Released 2022-10-21)

BUILD

- fix(deps): update module github.com/moov-io/base to v0.36.0
- fix(deps): update module github.com/prometheus/client_golang to v1.13.0
- fix(deps): update github.com/protonmail/go-crypto digest to c6815a8
- fix(deps): update module golang.org/x/crypto to v0.1.0

## v0.8.5 (Released 2022-07-27)

BUILD

- fix(deps): update golang.org/x/crypto digest to 630584e
- fix(deps): update module github.com/moov-io/base to v0.33.0
- fix(deps): update module github.com/stretchr/testify to v1.8.0
- fix(deps): update module github.com/pkg/sftp to v1.13.5
- fix(deps): update github.com/protonmail/go-crypto digest to e85cedf

## v0.8.4 (Released 2022-05-19)

BUILD

- fix(deps): update github.com/protonmail/go-crypto digest to 88bb529
- fix(deps): update golang.org/x/crypto digest to 6f7dac9

## v0.8.3 (Released 2022-05-16)

IMPROVEMENTS

- fix: log names of files found in ListFiles instead of `*FileInfo`

BUILD

- fix(deps): update golang.org/x/crypto digest to 403b017

## v0.8.2 (Released 2022-05-12)

BUILD

- build: require minimum test coverage again
- build: update ProtonMail/go-crypto, moov-io/base, and golang.org/x/crypto
- fix: tweak for staticcheck

## v0.8.1 (Released 2022-05-02)

BUILD

- fix(deps): update golang.org/x/crypto digest to eb4f295

## v0.8.0 (Released 2022-04-14)

ADDITIONS

- feat: new config flag to skip chmod after uploading a file

## v0.7.0 (Released 2022-03-25)

ADDITIONS

- feat: include ModTime on every File instance

BUILD

- fix(deps): update golang.org/x/crypto digest to 2c7772b

## v0.6.2 (Released 2022-03-22)

IMPROVEMENTS

- fix: return err if set on mock client file upload

## v0.6.1 (Released 2022-02-16)

IMPROVEMENTS

- feat: list files without opening

## v0.6.0 (Released 2022-02-04)

BREAKING CHANGES

- feat: our File type no longer implements the fs.File interface

IMPROVEMENTS

- fix: only log on client startup, not connection init
- feat: add enhanced debugging statements
- fix: close file descriptor after list

## v0.5.2 (Released 2022-01-31)

BUG FIXES

- client: allow multiple authentication methods

BUILD

- Update golang.org/x/crypto commit hash to 198e437
- Update module github.com/prometheus/client_golang to v1.12.1

## v0.5.1 (Released 2022-01-27)

BUG FIXES

- fix: bugfix for Read

## v0.5.0 (Released 2022-01-27)

ADDITIONS

- feat: make our File type fs.File compatible

BUILD

- Update golang.org/x/crypto commit hash to aa10faf

## v0.4.0 (Released 2022-01-25)

IMPROVEMENTS

- client: support reading private keys with passphrases

## v0.3.0 (Released 2022-01-21)

- feat: export MockClient
- refactor: use testing.T's TempDir instead of ioutil

## v0.2.0 (Released 2022-01-19)

- Added GitHub actions workflow
- Added new Prometheus metrics

## v0.1.2 (Released 2022-01-12)

- Updated filepaths in mock client to match the actual SFTP client

## v0.1.1 (Released 2022-01-6)

- Configured renovate bot for dependency updates

## v0.1.0 (Released 2022-01-6)

- Initial release
