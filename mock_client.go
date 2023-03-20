// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type MockClient struct {
	root string

	Err                 error
	Calls               int // count number of calls to Open(), Delete(), UploadFile(), ListFiles(), Walk()
	DesiredErrResponses int // stop returning Err after this many calls to Open(), Delete(), UploadFile(), ListFiles(), Walk()
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
	c.Calls++
	if c.Err != nil && c.DesiredErrResponses > 0 {
		return nil, c.Err
	}
	file, err := os.Open(filepath.Join(c.root, path))
	if err != nil {
		return nil, err
	}
	_, name := filepath.Split(path)
	return &File{
		Filename: name,
		Contents: file,
	}, nil
}

func (c *MockClient) Delete(path string) error {
	c.Calls++
	if c.Err != nil && c.DesiredErrResponses > 0 {
		c.DesiredErrResponses--
		return c.Err
	}
	return os.Remove(filepath.Join(c.root, path))
}

func (c *MockClient) UploadFile(path string, contents io.ReadCloser) error {
	c.Calls++
	if c.Err != nil && c.DesiredErrResponses > 0 {
		c.DesiredErrResponses--
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
	c.Calls++
	if c.Err != nil && c.DesiredErrResponses > 0 {
		return nil, c.Err
	}

	os.MkdirAll(filepath.Join(c.root, dir), 0777)

	fds, err := os.ReadDir(filepath.Join(c.root, dir))
	if err != nil {
		return nil, err
	}
	var out []string
	for i := range fds {
		fd := filepath.Join(dir, strings.TrimPrefix(fds[i].Name(), c.root))
		out = append(out, fd)
	}
	return out, nil
}

func (c *MockClient) Walk(dir string, fn fs.WalkDirFunc) error {
	c.Calls++
	if c.Err != nil && c.DesiredErrResponses > 0 {
		return c.Err
	}

	d, err := filepath.Abs(filepath.Join(c.root, dir))
	if err != nil {
		return err
	}
	os.MkdirAll(d, 0777)

	return fs.WalkDir(os.DirFS(d), ".", fn)
}
