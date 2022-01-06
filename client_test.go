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
