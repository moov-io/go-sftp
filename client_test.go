// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp_test

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moov-io/base/log"
	sftp "github.com/moov-io/go-sftp"

	"github.com/stretchr/testify/require"
)

func TestClientErr(t *testing.T) {
	client, err := sftp.NewClient(log.NewTestLogger(), nil)
	require.Error(t, err)
	require.Nil(t, client)

	_, err = sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
		Hostname:       "localhost:invalid",
		Timeout:        0 * time.Second,
		MaxConnections: 0,
		PacketSize:     0,
	})
	require.Error(t, err)
}

func TestClient(t *testing.T) {
	if testing.Short() {
		t.Skip("-short flag was provided")
	}

	client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
		Hostname:       "sftp:22",
		Username:       "demo",
		Password:       "password",
		Timeout:        5 * time.Second,
		MaxConnections: 1,
		PacketSize:     32000,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	t.Run("Ping", func(t *testing.T) {
		require.NoError(t, client.Ping())
	})

	t.Run("Open and Close", func(t *testing.T) {
		file, err := client.Open("/outbox/one.txt")
		require.NoError(t, err)
		require.Greater(t, file.ModTime.Unix(), int64(1e7)) // valid unix time

		content, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "one\n", string(content))

		require.NoError(t, file.Close())
	})

	t.Run("open larger files", func(t *testing.T) {
		largerFileSize := size(t, filepath.Join("testdata", "bigdata", "large.txt"))

		file, err := client.Open("/bigdata/large.txt")
		require.NoError(t, err)

		var buf bytes.Buffer
		_, err = io.Copy(&buf, file)
		require.NoError(t, err)

		require.NoError(t, file.Close())
		require.Len(t, buf.Bytes(), largerFileSize)
	})

	t.Run("Open with Reader and consume file", func(t *testing.T) {
		file, err := client.Reader("/outbox/one.txt")
		require.NoError(t, err)
		require.Greater(t, file.ModTime.Unix(), int64(1e7)) // valid unix time

		content, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "one\n", string(content))

		require.NoError(t, file.Close())
	})

	t.Run("read larger files", func(t *testing.T) {
		largerFileSize := size(t, filepath.Join("testdata", "bigdata", "large.txt"))

		file, err := client.Reader("/bigdata/large.txt")
		require.NoError(t, err)

		var buf bytes.Buffer
		_, err = io.Copy(&buf, file)
		require.NoError(t, err)

		require.NoError(t, file.Close())
		require.Len(t, buf.Bytes(), largerFileSize)
	})

	t.Run("ListFiles", func(t *testing.T) {
		files, err := client.ListFiles(".")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"root.txt"})

		files, err = client.ListFiles("/")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"/root.txt"})

		files, err = client.ListFiles("/outbox")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"/outbox/one.txt", "/outbox/two.txt", "/outbox/empty.txt"})

		files, err = client.ListFiles("outbox")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"outbox/one.txt", "outbox/two.txt", "outbox/empty.txt"})

		files, err = client.ListFiles("outbox/")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"outbox/one.txt", "outbox/two.txt", "outbox/empty.txt"})
	})

	t.Run("ListFiles subdir", func(t *testing.T) {
		files, err := client.ListFiles("/outbox/archive")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"/outbox/archive/empty2.txt", "/outbox/archive/three.txt"})

		files, err = client.ListFiles("outbox/archive")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"outbox/archive/empty2.txt", "outbox/archive/three.txt"})

		files, err = client.ListFiles("outbox/archive/")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"outbox/archive/empty2.txt", "outbox/archive/three.txt"})
	})

	t.Run("list and read", func(t *testing.T) {
		filenames, err := client.ListFiles("/outbox/with-empty")
		require.NoError(t, err)

		// randomize filename order
		rand.Shuffle(len(filenames), func(i, j int) {
			filenames[i], filenames[j] = filenames[j], filenames[i]
		})
		require.ElementsMatch(t, filenames, []string{
			"/outbox/with-empty/EMPTY1.txt", "/outbox/with-empty/empty_file2.txt",
			"/outbox/with-empty/data.txt", "/outbox/with-empty/data2.txt",
		})

		// read each file and get back expected contents
		var contents []string
		for i := range filenames {
			var file *sftp.File
			if i/2 == 0 {
				file, err = client.Open(filenames[i])
			} else {
				file, err = client.Reader(filenames[i])
			}
			require.NoError(t, err, "filenames[%d]", i)
			require.NotNil(t, file, "filenames[%d]", i)
			require.NotNil(t, file.Contents, "filenames[%d]", i)

			bs, err := io.ReadAll(file.Contents)
			require.NoError(t, err)

			contents = append(contents, string(bs))
		}

		require.ElementsMatch(t, contents, []string{"", "", "also data\n", "has data\n"})
	})

	t.Run("ListFiles case testing", func(t *testing.T) {
		files, err := client.ListFiles("/outbox/upper")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"/outbox/Upper/names.txt"})

		files, err = client.ListFiles("outbox/ARCHIVE")
		require.NoError(t, err)
		require.ElementsMatch(t, files, []string{"outbox/archive/empty2.txt", "outbox/archive/three.txt"})
	})

	t.Run("Walk", func(t *testing.T) {
		var walkedFiles []string
		err = client.Walk(".", func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if strings.Contains(path, "upload") {
				return fs.SkipDir
			}
			walkedFiles = append(walkedFiles, path)
			return nil
		})
		require.NoError(t, err)
		require.ElementsMatch(t, walkedFiles, []string{
			".", "root.txt",
			"bigdata", "bigdata/large.txt",
			"outbox", "outbox/Upper", "outbox/Upper/names.txt",
			"outbox/one.txt", "outbox/two.txt", "outbox/empty.txt",
			"outbox/archive", "outbox/archive/empty2.txt", "outbox/archive/three.txt",
			"outbox/with-empty", "outbox/with-empty/EMPTY1.txt", "outbox/with-empty/empty_file2.txt",
			"outbox/with-empty/data.txt", "outbox/with-empty/data2.txt",
		})
	})

	t.Run("Walk subdir", func(t *testing.T) {
		var walkedFiles []string
		err = client.Walk("/outbox", func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			walkedFiles = append(walkedFiles, path)
			return nil
		})
		require.NoError(t, err)
		require.ElementsMatch(t, walkedFiles, []string{
			"/outbox",
			"/outbox/Upper", "/outbox/Upper/names.txt",
			"/outbox/one.txt", "/outbox/two.txt", "/outbox/empty.txt",
			"/outbox/archive", "/outbox/archive/empty2.txt", "/outbox/archive/three.txt",
			"/outbox/with-empty",
			"/outbox/with-empty/EMPTY1.txt", "/outbox/with-empty/empty_file2.txt",
			"/outbox/with-empty/data.txt", "/outbox/with-empty/data2.txt",
		})
	})

	t.Run("Walk SkipDir", func(t *testing.T) {
		var walkedFiles []string
		err = client.Walk("/outbox", func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			walkedFiles = append(walkedFiles, path)
			return fs.SkipDir
		})
		require.NoError(t, err)
		require.ElementsMatch(t, walkedFiles, []string{"/outbox"})
	})

	t.Run("Upload and Delete", func(t *testing.T) {
		// upload file
		fileName := fmt.Sprintf("/upload/%d.txt", time.Now().Unix())
		err = client.UploadFile(fileName, io.NopCloser(bytes.NewBufferString("random")))
		require.NoError(t, err)

		// test uploaded file content
		file, err := client.Open(fileName)
		require.NoError(t, err)
		content, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "random", string(content))

		// delete file
		err = client.Delete(fileName)
		require.NoError(t, err)

		// test there is no file
		_, err = client.Open(fileName)
		require.EqualError(t, err, fmt.Sprintf("sftp: open %s: file does not exist", fileName))

		require.NoError(t, file.Close())
	})

	t.Run("Delete", func(t *testing.T) {
		err := client.Delete("/missing.txt")
		require.NoError(t, err)

		err = client.Delete("/no-existing-dir/missing.txt")
		require.NoError(t, err)
	})

	t.Run("Skip chmod after upload", func(t *testing.T) {
		// upload file
		fileName := fmt.Sprintf("/upload/%d.txt", time.Now().Unix())
		err = client.UploadFile(fileName, io.NopCloser(bytes.NewBufferString("random")))
		require.NoError(t, err)

		// test uploaded file content
		file, err := client.Open(fileName)
		require.NoError(t, err)
		content, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "random", string(content))

		// delete file
		err = client.Delete(fileName)
		require.NoError(t, err)
	})
}

func TestClient__UploadFile(t *testing.T) {
	if testing.Short() {
		t.Skip("-short flag was provided")
	}

	conf := &sftp.ClientConfig{
		Hostname:       "sftp:22",
		Username:       "demo",
		Password:       "password",
		Timeout:        5 * time.Second,
		MaxConnections: 1,
		PacketSize:     32000,
	}

	subdir := strconv.FormatInt(time.Now().UnixMilli(), 10)
	path := fmt.Sprintf("/upload/deep/nested/%s/file.txt", subdir)

	t.Run("don't create subdir", func(t *testing.T) {
		conf.SkipDirectoryCreation = true
		client, err := sftp.NewClient(log.NewTestLogger(), conf)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, client.Close())
		})

		contents := io.NopCloser(strings.NewReader("hello"))
		err = client.UploadFile(path, contents)
		require.ErrorContains(t, err, fmt.Sprintf("sftp: problem creating remote file %s: file does not exist", path))
	})

	t.Run("create subdir and upload", func(t *testing.T) {
		conf.SkipDirectoryCreation = false
		client, err := sftp.NewClient(log.NewTestLogger(), conf)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, client.Close())
		})

		contents := io.NopCloser(strings.NewReader("hello"))
		err = client.UploadFile(path, contents)
		require.NoError(t, err)

		// Cleanup
		require.NoError(t, client.Delete(path))
	})
}

func size(t *testing.T, where string) int {
	t.Helper()

	fd, err := os.Open(where)
	require.NoError(t, err)

	info, err := fd.Stat()
	require.NoError(t, err)

	return int(info.Size())
}
