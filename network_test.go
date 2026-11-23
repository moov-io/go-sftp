// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp_test

import (
	"testing"

	sftp "github.com/moov-io/go-sftp"
	"github.com/stretchr/testify/require"
)

func TestNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("-short flag was provided")
	}

	t.Run("in-process", func(t *testing.T) {
		runNetworkTests(t, testClient(t))
	})
	t.Run("openssh", func(t *testing.T) {
		runNetworkTests(t, openSSHClient(t))
	})
}

func runNetworkTests(t *testing.T, client sftp.Client) {
	t.Helper()

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
