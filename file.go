// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"io"
)

type File struct {
	Filename string
	Contents io.ReadCloser
}

func (f *File) Close() error {
	if f == nil {
		return nil
	}
	if f.Contents != nil {
		return f.Contents.Close()
	}
	return nil
}
