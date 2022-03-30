// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"io"
	"time"
)

type File struct {
	Filename string
	Contents io.ReadCloser

	// ModTime is a timestamp of when the last modification occurred
	// to this file. The default will be the current UTC time.
	ModTime time.Time
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
