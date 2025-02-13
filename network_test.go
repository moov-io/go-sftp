// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp_test

import (
	"testing"
	"time"

	"github.com/moov-io/base/log"
	sftp "github.com/moov-io/go-sftp"
	"github.com/stretchr/testify/require"
)

func TestNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("-short flag was provided")
	}

	client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
		Hostname:       "sftp:22",
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

	t.Run("Read after Closing", func(t *testing.T) {
		// Close the connection but have the caller try without knowing it's closed
		require.NoError(t, client.Close())

		files, err := client.ListFiles("/outbox")
		require.NoError(t, err)
		require.NotEmpty(t, files)

		// close it again for fun
		require.NoError(t, client.Close())

		// try again
		files, err = client.ListFiles("/outbox")
		require.NoError(t, err)
		require.NotEmpty(t, files)
	})
}
