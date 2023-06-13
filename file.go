// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"io"
	"io/fs"
	"time"
)

// File represents a fs.File object of a location on a SFTP server.
type File struct {
	Filename string
	Contents io.ReadCloser

	// ModTime is a timestamp of when the last modification occurred
	// to this file. The default will be the current UTC time.
	ModTime time.Time

	fileinfo fs.FileInfo
}

var _ fs.File = (&File{})

func (f *File) Close() error {
	if f == nil {
		return nil
	}
	if f.Contents != nil {
		return f.Contents.Close()
	}
	return nil
}

func (f *File) Stat() (fs.FileInfo, error) {
	if f == nil {
		return nil, io.EOF
	}
	return f.fileinfo, nil
}

func (f *File) Read(buf []byte) (int, error) {
	if f == nil || f.Contents == nil {
		return 0, io.EOF
	}
	return f.Contents.Read(buf)
}
