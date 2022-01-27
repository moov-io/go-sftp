// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFile(t *testing.T) {
	f := &File{}
	require.NoError(t, f.Close())
}
