// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFile(t *testing.T) {
	var f *File
	err := f.Close()
	require.NoError(t, err)
	stat, err := f.Stat()
	require.ErrorIs(t, err, io.EOF)
	require.Nil(t, stat)
	n, err := f.Read(nil)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 0, n)

	f = &File{}
	err = f.Close()
	require.NoError(t, err)
	stat, err = f.Stat()
	require.NoError(t, err)
	require.Nil(t, stat)
	n, err = f.Read(nil)
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 0, n)
}
