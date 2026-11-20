// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"io"
	"io/fs"
	"os"
	posixpath "path"
	"path/filepath"
	"strings"
	"testing"
)

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

func (c *MockClient) local(p string) string {
	p = posixpath.Clean("/" + strings.TrimPrefix(filepath.ToSlash(p), "/"))
	rel := strings.TrimPrefix(p, "/")
	if rel == "" || rel == "." {
		return c.root
	}
	return filepath.Join(c.root, filepath.FromSlash(rel))
}

func (c *MockClient) Open(path string) (*File, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	file, err := os.Open(c.local(path))
	if err != nil {
		return nil, err
	}
	return &File{
		Filename: posixpath.Base(filepath.ToSlash(path)),
		Contents: file,
	}, nil
}

func (c *MockClient) Delete(path string) error {
	return os.Remove(c.local(path))
}

func (c *MockClient) UploadFile(path string, contents io.ReadCloser) error {
	if c.Err != nil {
		return c.Err
	}

	dir := posixpath.Dir(filepath.ToSlash(path))
	if dir != "." && dir != "/" {
		if err := os.MkdirAll(c.local(dir), 0777); err != nil {
			return err
		}
	}

	bs, _ := io.ReadAll(contents)

	return os.WriteFile(c.local(path), bs, 0600)
}

func (c *MockClient) ListFiles(dir string) ([]string, error) {
	if c.Err != nil {
		return nil, c.Err
	}

	os.MkdirAll(c.local(dir), 0777)

	fds, err := os.ReadDir(c.local(dir))
	if err != nil {
		return nil, err
	}
	var out []string
	for i := range fds {
		out = append(out, posixpath.Join(filepath.ToSlash(dir), fds[i].Name()))
	}
	return out, nil
}

func (c *MockClient) Walk(dir string, fn fs.WalkDirFunc) error {
	if c.Err != nil {
		return c.Err
	}

	d, err := filepath.Abs(c.local(dir))
	if err != nil {
		return err
	}
	os.MkdirAll(d, 0777)

	return fs.WalkDir(os.DirFS(d), ".", fn)
}
