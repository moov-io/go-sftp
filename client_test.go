// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp_test

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/moov-io/base/log"
	sftp "github.com/moov-io/go-sftp"

	"github.com/stretchr/testify/require"
)

func TestClientErr(t *testing.T) {
	client, err := sftp.NewClient(log.NewTestLogger(), nil)
	require.Error(t, err)
	require.Nil(t, client)

	_, err = sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
		Hostname:       "localhost:invalid",
		Timeout:        0 * time.Second,
		MaxConnections: 0,
		PacketSize:     0,
	})
	require.Error(t, err)
}

func TestClient(t *testing.T) {
	if testing.Short() {
		t.Skip("-short flag was provided")
	}

	client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
		Hostname:       "localhost:2222",
		Username:       "demo",
		Password:       "password",
		Timeout:        5 * time.Second,
		MaxConnections: 1,
		PacketSize:     32000,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	t.Run("Ping", func(t *testing.T) {
		require.NoError(t, client.Ping())
	})

	t.Run("Open and Close", func(t *testing.T) {
		file, err := client.Open("/outbox/one.txt")
		require.NoError(t, err)
		require.Greater(t, file.ModTime.Unix(), int64(1e7)) // valid unix time

		content, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "one\n", string(content))

		require.NoError(t, file.Close())
	})

	t.Run("Open with Reader and consume file", func(t *testing.T) {
		file, err := client.Reader("/outbox/one.txt")
		require.NoError(t, err)
		require.Greater(t, file.ModTime.Unix(), int64(1e7)) // valid unix time

		content, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "one\n", string(content))

		require.NoError(t, file.Close())
	})

	t.Run("ListFiles", func(t *testing.T) {
		files, err := client.ListFiles("/")
		require.NoError(t, err)
		require.Len(t, files, 0)

		files, err = client.ListFiles("/outbox")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"/outbox/one.txt", "/outbox/two.txt", "/outbox/empty.txt"})

		files, err = client.ListFiles("outbox")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"outbox/one.txt", "outbox/two.txt", "outbox/empty.txt"})

		files, err = client.ListFiles("outbox/")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"outbox/one.txt", "outbox/two.txt", "outbox/empty.txt"})
	})

	t.Run("ListFiles subdir", func(t *testing.T) {
		files, err := client.ListFiles("/outbox/archive")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"/outbox/archive/empty2.txt", "/outbox/archive/three.txt"})

		files, err = client.ListFiles("outbox/archive")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"outbox/archive/empty2.txt", "outbox/archive/three.txt"})

		files, err = client.ListFiles("outbox/archive/")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"outbox/archive/empty2.txt", "outbox/archive/three.txt"})
	})

	t.Run("Walk", func(t *testing.T) {
		var walkedFiles []string
		err = client.Walk(".", func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			walkedFiles = append(walkedFiles, path)
			return nil
		})
		require.NoError(t, err)
		require.ElementsMatch(t, walkedFiles, []string{
			"outbox/one.txt", "outbox/two.txt", "outbox/empty.txt",
			"outbox/archive/empty2.txt", "outbox/archive/three.txt",
		})
	})

	t.Run("Walk subdir", func(t *testing.T) {
		var walkedFiles []string
		err = client.Walk("/outbox", func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			walkedFiles = append(walkedFiles, path)
			return nil
		})
		require.NoError(t, err)
		require.ElementsMatch(t, walkedFiles, []string{
			"/outbox/one.txt", "/outbox/two.txt", "/outbox/empty.txt",
			"/outbox/archive/empty2.txt", "/outbox/archive/three.txt",
		})
	})

	t.Run("Upload and Delete", func(t *testing.T) {
		// upload file
		fileName := fmt.Sprintf("/upload/%d.txt", time.Now().Unix())
		err = client.UploadFile(fileName, io.NopCloser(bytes.NewBufferString("random")))
		require.NoError(t, err)

		// test uploaded file content
		file, err := client.Open(fileName)
		require.NoError(t, err)
		content, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "random", string(content))

		// delete file
		err = client.Delete(fileName)
		require.NoError(t, err)

		// test there is no file
		_, err = client.Open(fileName)
		require.EqualError(t, err, fmt.Sprintf("sftp: open %s: file does not exist", fileName))

		require.NoError(t, file.Close())
	})

	t.Run("Skip chmod after upload", func(t *testing.T) {
		// upload file
		fileName := fmt.Sprintf("/upload/%d.txt", time.Now().Unix())
		err = client.UploadFile(fileName, io.NopCloser(bytes.NewBufferString("random")))
		require.NoError(t, err)

		// test uploaded file content
		file, err := client.Open(fileName)
		require.NoError(t, err)
		content, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "random", string(content))

		// delete file
		err = client.Delete(fileName)
		require.NoError(t, err)
	})
}

func TestClient__UploadFile(t *testing.T) {
	if testing.Short() {
		t.Skip("-short flag was provided")
	}

	conf := &sftp.ClientConfig{
		Hostname:       "localhost:2222",
		Username:       "demo",
		Password:       "password",
		Timeout:        5 * time.Second,
		MaxConnections: 1,
		PacketSize:     32000,
	}

	subdir := fmt.Sprintf("%d", time.Now().UnixMilli())
	path := fmt.Sprintf("/upload/deep/nested/%s/file.txt", subdir)

	t.Run("don't create subdir", func(t *testing.T) {
		conf.SkipDirectoryCreation = true
		client, err := sftp.NewClient(log.NewTestLogger(), conf)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, client.Close())
		})

		contents := io.NopCloser(strings.NewReader("hello"))
		err = client.UploadFile(path, contents)
		require.ErrorContains(t, err, fmt.Sprintf("sftp: problem creating remote file %s: file does not exist", path))
	})

	t.Run("create subdir and upload", func(t *testing.T) {
		conf.SkipDirectoryCreation = false
		client, err := sftp.NewClient(log.NewTestLogger(), conf)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, client.Close())
		})

		contents := io.NopCloser(strings.NewReader("hello"))
		err = client.UploadFile(path, contents)
		require.NoError(t, err)

		// Cleanup
		require.NoError(t, client.Delete(path))
	})
}
