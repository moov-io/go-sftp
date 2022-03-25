// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"testing"
	"time"

	"github.com/moov-io/base/log"
	sftp "github.com/moov-io/go-sftp"

	"github.com/stretchr/testify/require"
)

func TestClientErr(t *testing.T) {
	client, err := sftp.NewClient(log.NewNopLogger(), nil)
	require.Error(t, err)
	require.Nil(t, client)

	_, err = sftp.NewClient(log.NewNopLogger(), &sftp.ClientConfig{
		Hostname:       "localhost:invalid",
		Timeout:        0 * time.Second,
		MaxConnections: 0,
		PacketSize:     0,
	})
	require.Error(t, err)
}

func TestClient_New(t *testing.T) {
	t.Run("Open and Close", func(t *testing.T) {
		client, err := sftp.NewClient(log.NewNopLogger(), &sftp.ClientConfig{
			Hostname:       "localhost:2222",
			Username:       "demo",
			Password:       "password",
			Timeout:        5 * time.Second,
			MaxConnections: 1,
			PacketSize:     32000,
		})
		require.NoError(t, err)

		file, err := client.Open("/outbox/one.txt")
		require.NoError(t, err)
		require.Greater(t, file.ModTime.Unix(), int64(1e7)) // valid unix time

		content, err := ioutil.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "one\n", string(content))

		require.NoError(t, file.Close())
		require.NoError(t, client.Close())
	})

	t.Run("ListFiles", func(t *testing.T) {
		client, err := sftp.NewClient(log.NewNopLogger(), &sftp.ClientConfig{
			Hostname:       "localhost:2222",
			Username:       "demo",
			Password:       "password",
			Timeout:        5 * time.Second,
			MaxConnections: 1,
			PacketSize:     32000,
		})
		require.NoError(t, err)

		files, err := client.ListFiles("/outbox")

		require.NoError(t, err)
		require.Equal(t, []string{"/outbox/one.txt", "/outbox/two.txt"}, files)

		require.NoError(t, client.Close())
	})

	t.Run("Ping", func(t *testing.T) {
		client, err := sftp.NewClient(log.NewNopLogger(), &sftp.ClientConfig{
			Hostname:       "localhost:2222",
			Username:       "demo",
			Password:       "password",
			Timeout:        5 * time.Second,
			MaxConnections: 1,
			PacketSize:     32000,
		})
		require.NoError(t, err)

		require.NoError(t, client.Ping())
	})

	t.Run("Upload and Delete", func(t *testing.T) {
		client, err := sftp.NewClient(log.NewNopLogger(), &sftp.ClientConfig{
			Hostname:       "localhost:2222",
			Username:       "demo",
			Password:       "password",
			Timeout:        5 * time.Second,
			MaxConnections: 1,
			PacketSize:     32000,
		})
		require.NoError(t, err)

		// upload file
		fileName := fmt.Sprintf("/upload/%d.txt", time.Now().Unix())
		err = client.UploadFile(fileName, io.NopCloser(bytes.NewBufferString("random")))
		require.NoError(t, err)

		// test uploaded file content
		file, err := client.Open(fileName)
		require.NoError(t, err)
		content, err := ioutil.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "random", string(content))

		// delete file
		err = client.Delete(fileName)
		require.NoError(t, err)

		// test there is no file
		_, err = client.Open(fileName)
		require.EqualError(t, err, fmt.Sprintf("sftp: open %s: file does not exist", fileName))

		require.NoError(t, file.Close())
		require.NoError(t, client.Close())
	})
}
