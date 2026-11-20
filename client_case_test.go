// Copyright 2026 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp_test

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/moov-io/base/log"
	sftp "github.com/moov-io/go-sftp"
	"github.com/moov-io/go-sftp/internal/sftptest"
	"github.com/stretchr/testify/require"
)

func TestClient_CaseSensitivity(t *testing.T) {
	if testing.Short() {
		t.Skip("-short flag was provided")
	}

	t.Run("os", func(t *testing.T) {
		srv := startFixtureServer(t)
		runCaseTests(t, newClientFromServer(t, srv), caseInsensitiveFS(t, srv.Root))
	})

	t.Run("in-memory", func(t *testing.T) {
		srv := sftptest.StartInMemory(t)
		client := newClientFromServer(t, srv)
		require.NoError(t, client.UploadFile("/outbox/Upper/names.txt", io.NopCloser(strings.NewReader("adam\n"))))
		require.NoError(t, client.UploadFile("/outbox/one.txt", io.NopCloser(strings.NewReader("one\n"))))
		require.NoError(t, client.UploadFile("/outbox/archive/three.txt", io.NopCloser(strings.NewReader("three\n"))))
		runCaseTests(t, client, false)
	})
}

func runCaseTests(t *testing.T, client sftp.Client, caseInsensitive bool) {
	t.Helper()

	t.Run("ListFiles is case-insensitive and preserves server case", func(t *testing.T) {
		files, err := client.ListFiles("/outbox/upper")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"/outbox/Upper/names.txt"})
		for _, name := range files {
			require.NotContains(t, name, `\`)
		}

		files, err = client.ListFiles("outbox/ARCHIVE")
		require.NoError(t, err)
		require.Contains(t, files, "outbox/archive/three.txt")
		for _, name := range files {
			require.True(t, strings.HasPrefix(strings.ToLower(name), "outbox/archive/"))
		}
	})

	t.Run("Open mismatched case", func(t *testing.T) {
		file, err := client.Open("/outbox/UPPER/names.txt")
		if caseInsensitive {
			require.NoError(t, err)
			body, err := io.ReadAll(file.Contents)
			require.NoError(t, err)
			require.Equal(t, "adam\n", string(body))
			require.NoError(t, file.Close())
			return
		}
		require.Error(t, err)

		file, err = client.Open("/outbox/Upper/names.txt")
		require.NoError(t, err)
		body, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "adam\n", string(body))
		require.NoError(t, file.Close())
	})

	t.Run("files that differ only by case", func(t *testing.T) {
		require.NoError(t, client.UploadFile("/casedir/Readme.txt", io.NopCloser(strings.NewReader("title"))))
		require.NoError(t, client.UploadFile("/casedir/README.txt", io.NopCloser(strings.NewReader("TITLE"))))

		files, err := client.ListFiles("/casedir")
		require.NoError(t, err)
		for _, name := range files {
			require.NotContains(t, name, `\`)
		}

		if caseInsensitive {
			require.Len(t, files, 1)
			file, err := client.Open("/casedir/readme.txt")
			require.NoError(t, err)
			require.NoError(t, file.Close())
			return
		}

		require.ElementsMatch(t, files, []string{"/casedir/Readme.txt", "/casedir/README.txt"})

		file, err := client.Open("/casedir/Readme.txt")
		require.NoError(t, err)
		body, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "title", string(body))
		require.NoError(t, file.Close())

		file, err = client.Open("/casedir/README.txt")
		require.NoError(t, err)
		body, err = io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "TITLE", string(body))
		require.NoError(t, file.Close())

		_, err = client.Open("/casedir/readme.txt")
		require.Error(t, err)
	})
}

func TestClient_HostKey(t *testing.T) {
	if testing.Short() {
		t.Skip("-short flag was provided")
	}

	srv := startFixtureServer(t)

	t.Run("match", func(t *testing.T) {
		require.NoError(t, newClientFromServer(t, srv).Ping())
	})

	t.Run("mismatch", func(t *testing.T) {
		_, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
			Hostname:       srv.Addr,
			Username:       sftptest.Username,
			Password:       sftptest.Password,
			Timeout:        5 * time.Second,
			MaxConnections: 1,
			HostPublicKeys: []string{ed25519Key},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "no matching host keys")
	})
}

func TestClient_ListFilesSlashSeparated(t *testing.T) {
	if testing.Short() {
		t.Skip("-short flag was provided")
	}

	client := testClient(t)
	files, err := client.ListFiles("/outbox")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, name := range files {
		require.NotContains(t, name, `\`)
		require.True(t, strings.HasPrefix(name, "/outbox/"))
	}
}
