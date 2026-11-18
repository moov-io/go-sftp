// Copyright 2026 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"io"
	"io/fs"
	"path"
	"testing"

	"github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
)

type pipeConn struct {
	io.Reader
	io.WriteCloser
}

func newInMemoryClient(t *testing.T, files ...string) *client {
	t.Helper()

	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	server := sftp.NewRequestServer(pipeConn{serverReader, serverWriter}, sftp.InMemHandler())
	go server.Serve()

	conn, err := sftp.NewClientPipe(clientReader, clientWriter)
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Close()
		conn.Close()
	})

	for _, name := range files {
		require.NoError(t, conn.MkdirAll(path.Dir(name)))
		fd, err := conn.Create(name)
		require.NoError(t, err)
		require.NoError(t, fd.Close())
	}

	return &client{client: conn}
}

func TestWalk_SkipDirOnlySkipsThatDirectory(t *testing.T) {
	c := newInMemoryClient(t,
		"/outbox/archive/old.txt",
		"/outbox/archive2/kept.txt",
		"/outbox/one.txt",
		"/outbox/with-empty/data.txt",
	)

	var walked []string
	err := c.Walk("/outbox", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "/outbox/archive" {
			return fs.SkipDir
		}
		walked = append(walked, p)
		return nil
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"/outbox",
		"/outbox/archive2", "/outbox/archive2/kept.txt",
		"/outbox/one.txt",
		"/outbox/with-empty", "/outbox/with-empty/data.txt",
	}, walked)
}

func TestWalk_SkipDirOnFileSkipsRemainingSiblings(t *testing.T) {
	c := newInMemoryClient(t,
		"/outbox/a/1.txt",
		"/outbox/a/2.txt",
		"/outbox/b/3.txt",
	)

	var walked []string
	err := c.Walk("/outbox", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		walked = append(walked, p)
		if p == "/outbox/a/1.txt" {
			return fs.SkipDir
		}
		return nil
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"/outbox",
		"/outbox/a", "/outbox/a/1.txt",
		"/outbox/b", "/outbox/b/3.txt",
	}, walked)
}
