// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/pkg/sftp"
)

type File struct {
	fd *sftp.File

	Filename string
	Contents io.ReadCloser
}

func (f *File) Read(b []byte) (int, error) {
	if f == nil {
		return 0, io.EOF
	}
	return f.fd.Read(b)
}

func (f *File) Stat() (os.FileInfo, error) {
	if f == nil {
		return nil, errors.New("nil File")
	}
	return f.fd.Stat()
}

func (f *File) Close() error {
	if f.Contents != nil {
		f.Contents.Close()
	}
	if f.fd != nil {
		return f.fd.Close()
	}
	return nil
}

var _ fs.File = (&File{})
