// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"
)

// MockClient is an in-process Client that stores files in a temporary
// directory. It is intended for tests. NewMockClient requires a *testing.T.
type MockClient struct {
	root string

	Err error
}

var _ Client = (&MockClient{})

func NewMockClient(t *testing.T) *MockClient {
	return &MockClient{
		root: t.TempDir(),
	}
}

func (c *MockClient) Ping() error {
	return c.Err
}

func (c *MockClient) Dir() string {
	return c.root
}

func (c *MockClient) Close() error {
	return c.Err
}

func (c *MockClient) Reader(path string) (*File, error) {
	return c.Open(path)
}

func (c *MockClient) Open(path string) (*File, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	file, err := os.Open(filepath.Join(c.root, path))
	if err != nil {
		return nil, err
	}
	info, statErr := file.Stat()
	modTime := time.Time{}
	if statErr == nil && info != nil {
		modTime = info.ModTime()
	}
	return &File{
		Filename: path,
		Contents: file,
		ModTime:  modTime,
		fileinfo: info,
	}, nil
}

func (c *MockClient) Delete(path string) error {
	if c.Err != nil {
		return c.Err
	}
	err := os.Remove(filepath.Join(c.root, path))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (c *MockClient) UploadFile(path string, contents io.ReadCloser) error {
	if c.Err != nil {
		return c.Err
	}

	dir, _ := filepath.Split(path)
	if err := os.MkdirAll(filepath.Join(c.root, dir), 0777); err != nil {
		return err
	}

	bs, _ := io.ReadAll(contents)

	return os.WriteFile(filepath.Join(c.root, path), bs, 0600)
}

func (c *MockClient) ListFiles(dir string) ([]string, error) {
	if c.Err != nil {
		return nil, c.Err
	}

	local := filepath.Join(c.root, filepath.FromSlash(dir))
	if err := os.MkdirAll(local, 0777); err != nil {
		return nil, err
	}

	fds, err := os.ReadDir(local)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, fd := range fds {
		if fd.IsDir() {
			continue
		}
		out = append(out, path.Join(dir, fd.Name()))
	}
	return out, nil
}

func (c *MockClient) Walk(dir string, fn fs.WalkDirFunc) error {
	if c.Err != nil {
		return c.Err
	}

	d, err := filepath.Abs(filepath.Join(c.root, dir))
	if err != nil {
		return err
	}
	os.MkdirAll(d, 0777)

	return fs.WalkDir(os.DirFS(d), ".", fn)
}
