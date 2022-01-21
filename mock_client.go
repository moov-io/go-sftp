// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockClient struct {
	root string

	Err error
}

func NewMockClient(t *testing.T) *mockClient {
	return &mockClient{
		root: t.TempDir(),
	}
}

func (c *mockClient) Ping() error {
	return c.Err
}

func (c *mockClient) Dir() string {
	return c.root
}

func (c *mockClient) Close() error {
	return c.Err
}

func (c *mockClient) Open(path string) (*File, error) {
	if c.Err != nil {
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

func (c *mockClient) Delete(path string) error {
	return os.Remove(filepath.Join(c.root, path))
}

func (c *mockClient) UploadFile(path string, contents io.ReadCloser) error {
	dir, _ := filepath.Split(path)
	if err := os.MkdirAll(filepath.Join(c.root, dir), 0777); err != nil {
		return err
	}

	bs, _ := ioutil.ReadAll(contents)

	return ioutil.WriteFile(filepath.Join(c.root, path), bs, 0600)
}

func (c *mockClient) ListFiles(dir string) ([]string, error) {
	if c.Err != nil {
		return nil, c.Err
	}

	os.MkdirAll(filepath.Join(c.root, dir), 0777)

	fds, err := ioutil.ReadDir(filepath.Join(c.root, dir))
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
