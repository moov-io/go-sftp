// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp_test

import (
	"errors"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	sftp "github.com/moov-io/go-sftp"

	"github.com/stretchr/testify/require"
)

func TestMockClient(t *testing.T) {
	client := sftp.NewMockClient(t)
	require.NoError(t, client.Ping())
	defer require.NoError(t, client.Close())

	_, err := client.Open("/missing.txt")
	require.Error(t, err)

	err = client.Delete("/missing.txt")
	require.Error(t, err)

	body := ioutil.NopCloser(strings.NewReader("contents"))
	err = client.UploadFile("/exists.txt", body)
	require.NoError(t, err)

	client.Err = errors.New("bad error")
	err = client.UploadFile("/exists.txt", body)
	require.Error(t, err)

	// reset mock client err
	client.Err = nil

	paths, err := client.ListFiles("/")
	require.NoError(t, err)
	require.Len(t, paths, 1)
	require.Equal(t, "/exists.txt", paths[0])
}

func TestMockClient_ListAndOpenFiles(t *testing.T) {
	client := sftp.NewMockClient(t)
	require.NoError(t, client.Ping())
	defer require.NoError(t, client.Close())

	require.NoError(t, client.UploadFile("/path/f1.txt", ioutil.NopCloser(strings.NewReader("foo"))))
	require.NoError(t, client.UploadFile("/path/f2.txt", ioutil.NopCloser(strings.NewReader("foo"))))

	foundFiles, err := client.ListFiles("/path/")
	require.NoError(t, err)
	require.Len(t, foundFiles, 2)

	for _, file := range foundFiles {
		found, err := client.Open(file)
		require.NoError(t, err)
		require.NotNil(t, found)

		contents, err := io.ReadAll(found.Contents)
		require.NoError(t, err)
		require.Equal(t, "foo", string(contents))
	}
}
