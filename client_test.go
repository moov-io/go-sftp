// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moov-io/base/log"
	sftp "github.com/moov-io/go-sftp"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestClientErr(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		client, err := sftp.NewClient(log.NewTestLogger(), nil)
		require.Error(t, err)
		require.Nil(t, client)
		require.EqualError(t, err, "nil SFTP config")
	})

	t.Run("invalid hostname still returns client", func(t *testing.T) {
		client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
			Hostname:       "localhost:invalid",
			Timeout:        0 * time.Second,
			MaxConnections: 0,
			PacketSize:     0,
		})
		require.Error(t, err)
		require.NotNil(t, client)
		require.NotContains(t, err.Error(), "n must be greater or equal to 1")
		require.Error(t, client.Ping())
		files, listErr := client.ListFiles(".")
		require.Error(t, listErr)
		require.Empty(t, files)
		_, openErr := client.Open("missing.txt")
		require.Error(t, openErr)
		_, readerErr := client.Reader("missing.txt")
		require.Error(t, readerErr)
		require.Error(t, client.UploadFile("missing.txt", io.NopCloser(strings.NewReader("x"))))
		require.Error(t, client.Delete("missing.txt"))
		require.Error(t, client.Walk(".", func(string, fs.DirEntry, error) error { return nil }))
		require.NoError(t, client.Close())
	})

	t.Run("connection refused still returns client", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := ln.Addr().String()
		require.NoError(t, ln.Close())

		client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
			Hostname:       addr,
			Timeout:        time.Second,
			MaxConnections: 0,
		})
		require.Error(t, err)
		require.NotNil(t, client)
		require.NotContains(t, err.Error(), "n must be greater or equal to 1")
		require.Error(t, client.Ping())
		require.NoError(t, client.Close())
	})

	t.Run("invalid host public key", func(t *testing.T) {
		client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
			Hostname:       "127.0.0.1:1",
			HostPublicKeys: []string{"not-a-host-key"},
		})
		require.Error(t, err)
		require.NotNil(t, client)
		require.ErrorContains(t, err, "reading host key")
		require.NoError(t, client.Close())
	})

	t.Run("invalid client private key", func(t *testing.T) {
		client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
			Hostname:         "127.0.0.1:1",
			ClientPrivateKey: "not-a-private-key",
		})
		require.Error(t, err)
		require.NotNil(t, client)
		require.ErrorContains(t, err, "client private key")
		require.NoError(t, client.Close())
	})

	t.Run("encrypted private key with wrong passphrase", func(t *testing.T) {
		key := newClientRSAKey(t)
		client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
			Hostname:                 "127.0.0.1:1",
			ClientPrivateKey:         privateKeyPEM(t, key, "correct-secret"),
			ClientPrivateKeyPassword: "wrong-secret",
		})
		require.Error(t, err)
		require.NotNil(t, client)
		require.ErrorContains(t, err, "client private key")
		require.NoError(t, client.Close())
	})

	t.Run("against local server", func(t *testing.T) {
		root := t.TempDir()
		writeLocalTree(t, root, map[string]string{"root.txt": "ok"})
		addr, hostKey := startLocalSFTPServer(t, root)

		connect := func(t *testing.T, mutate func(*sftp.ClientConfig)) (sftp.Client, error) {
			t.Helper()
			cfg := &sftp.ClientConfig{
				Hostname:       addr,
				Username:       "demo",
				Password:       "password",
				Timeout:        5 * time.Second,
				MaxConnections: 1,
				HostPublicKeys: []string{hostKey},
			}
			if mutate != nil {
				mutate(cfg)
			}
			return sftp.NewClient(log.NewTestLogger(), cfg)
		}

		requireClient := func(t *testing.T, mutate func(*sftp.ClientConfig)) sftp.Client {
			t.Helper()
			client, err := connect(t, mutate)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, client.Close())
			})
			return client
		}

		t.Run("max connections zero", func(t *testing.T) {
			// MaxConnections 0 makes pkg/sftp reject the client after SSH connects.
			// Intentionally preserved for compatibility.
			client, err := connect(t, func(cfg *sftp.ClientConfig) {
				cfg.MaxConnections = 0
				cfg.PacketSize = 0
			})
			require.Error(t, err)
			require.NotNil(t, client)
			require.ErrorContains(t, err, "n must be greater or equal to 1")
			require.NoError(t, client.Close())
		})

		t.Run("max connections negative", func(t *testing.T) {
			client, err := connect(t, func(cfg *sftp.ClientConfig) {
				cfg.MaxConnections = -3
			})
			require.Error(t, err)
			require.NotNil(t, client)
			require.ErrorContains(t, err, "n must be greater or equal to 1")
			require.NoError(t, client.Close())
		})

		t.Run("does not mutate caller config", func(t *testing.T) {
			cfg := &sftp.ClientConfig{
				Hostname:       addr,
				Username:       "demo",
				Password:       "password",
				Timeout:        5 * time.Second,
				MaxConnections: 0,
				PacketSize:     0,
				HostPublicKeys: []string{hostKey},
			}
			client, err := sftp.NewClient(log.NewTestLogger(), cfg)
			require.Error(t, err)
			require.NotNil(t, client)
			t.Cleanup(func() {
				require.NoError(t, client.Close())
			})
			require.Equal(t, 0, cfg.MaxConnections)
			require.Equal(t, 0, cfg.PacketSize)
		})

		t.Run("packet size", func(t *testing.T) {
			client := requireClient(t, func(cfg *sftp.ClientConfig) {
				cfg.PacketSize = 32768
			})
			require.NoError(t, client.Ping())
		})

		t.Run("packet size too large", func(t *testing.T) {
			client, err := connect(t, func(cfg *sftp.ClientConfig) {
				cfg.PacketSize = 40000
			})
			require.Error(t, err)
			require.NotNil(t, client)
			require.ErrorContains(t, err, "32KB")
			require.NoError(t, client.Close())
		})

		t.Run("nil logger", func(t *testing.T) {
			client, err := sftp.NewClient(nil, &sftp.ClientConfig{
				Hostname:       addr,
				Username:       "demo",
				Password:       "password",
				Timeout:        5 * time.Second,
				MaxConnections: 1,
				HostPublicKeys: []string{hostKey},
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, client.Close())
			})
			require.NoError(t, client.Ping())
		})

		t.Run("HostPublicKey", func(t *testing.T) {
			client := requireClient(t, func(cfg *sftp.ClientConfig) {
				cfg.HostPublicKeys = nil
				cfg.HostPublicKey = hostKey
			})
			require.NoError(t, client.Ping())
		})

		t.Run("HostPublicKeys extra unused", func(t *testing.T) {
			client := requireClient(t, func(cfg *sftp.ClientConfig) {
				cfg.HostPublicKeys = []string{mismatchKey, hostKey}
			})
			require.NoError(t, client.Ping())
		})

		t.Run("wrong password", func(t *testing.T) {
			client, err := connect(t, func(cfg *sftp.ClientConfig) {
				cfg.Password = "nope"
			})
			require.Error(t, err)
			require.NotNil(t, client)
			require.Error(t, client.Ping())
			require.NoError(t, client.Close())
		})

		t.Run("wrong username", func(t *testing.T) {
			client, err := connect(t, func(cfg *sftp.ClientConfig) {
				cfg.Username = "other"
			})
			require.Error(t, err)
			require.NotNil(t, client)
			require.NoError(t, client.Close())
		})

		t.Run("host key mismatch", func(t *testing.T) {
			client, err := connect(t, func(cfg *sftp.ClientConfig) {
				cfg.HostPublicKeys = []string{mismatchKey}
			})
			require.Error(t, err)
			require.NotNil(t, client)
			require.ErrorContains(t, err, "no matching host keys")
			require.NoError(t, client.Close())
		})

		t.Run("empty host keys", func(t *testing.T) {
			client := requireClient(t, func(cfg *sftp.ClientConfig) {
				cfg.HostPublicKeys = nil
			})
			require.NoError(t, client.Ping())
		})
	})

	t.Run("public key auth", func(t *testing.T) {
		root := t.TempDir()
		writeLocalTree(t, root, map[string]string{"root.txt": "ok"})
		key := newClientRSAKey(t)
		addr, hostKey := startLocalSFTPServerAuth(t, root, clientPublicKey(t, key))

		client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
			Hostname:         addr,
			Username:         "demo",
			Timeout:          5 * time.Second,
			MaxConnections:   1,
			HostPublicKeys:   []string{hostKey},
			ClientPrivateKey: privateKeyPEM(t, key, ""),
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, client.Close())
		})
		require.NoError(t, client.Ping())
		files, err := client.ListFiles(".")
		require.NoError(t, err)
		require.Equal(t, []string{"root.txt"}, files)
	})

	t.Run("public key auth base64", func(t *testing.T) {
		root := t.TempDir()
		writeLocalTree(t, root, map[string]string{"root.txt": "ok"})
		key := newClientRSAKey(t)
		addr, hostKey := startLocalSFTPServerAuth(t, root, clientPublicKey(t, key))
		pem := privateKeyPEM(t, key, "")

		client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
			Hostname:         addr,
			Username:         "demo",
			Timeout:          5 * time.Second,
			MaxConnections:   1,
			HostPublicKeys:   []string{hostKey},
			ClientPrivateKey: base64.StdEncoding.EncodeToString([]byte(pem)),
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, client.Close())
		})
		require.NoError(t, client.Ping())
	})

	t.Run("public key auth with passphrase", func(t *testing.T) {
		root := t.TempDir()
		writeLocalTree(t, root, map[string]string{"root.txt": "ok"})
		key := newClientRSAKey(t)
		addr, hostKey := startLocalSFTPServerAuth(t, root, clientPublicKey(t, key))

		client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
			Hostname:                 addr,
			Username:                 "demo",
			Timeout:                  5 * time.Second,
			MaxConnections:           1,
			HostPublicKeys:           []string{hostKey},
			ClientPrivateKey:         privateKeyPEM(t, key, "passphrase"),
			ClientPrivateKeyPassword: "passphrase",
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, client.Close())
		})
		require.NoError(t, client.Ping())
	})

	t.Run("retries after server becomes available", func(t *testing.T) {
		root := t.TempDir()
		writeLocalTree(t, root, map[string]string{"root.txt": "ok"})

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := ln.Addr().String()
		signer := newHostSigner(t)
		hostKey := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))

		var ready atomic.Bool
		serveLocalSFTPWhen(t, ln, signer, root, nil, &ready)

		client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
			Hostname:       addr,
			Username:       "demo",
			Password:       "password",
			Timeout:        5 * time.Second,
			MaxConnections: 1,
			HostPublicKeys: []string{hostKey},
		})
		require.Error(t, err)
		require.NotNil(t, client)
		require.NotContains(t, err.Error(), "n must be greater or equal to 1")

		ready.Store(true)

		require.NoError(t, client.Ping())
		files, err := client.ListFiles(".")
		require.NoError(t, err)
		require.Equal(t, []string{"root.txt"}, files)
		require.NoError(t, client.Close())
	})
}

func TestClient(t *testing.T) {
	if testing.Short() {
		t.Skip("-short flag was provided")
	}

	client, err := sftp.NewClient(log.NewTestLogger(), &sftp.ClientConfig{
		Hostname:       "localhost:2222",
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

		files, err = client.ListFiles("onlydir")
		require.NoError(t, err)
		require.Empty(t, files)

		files, err = client.ListFiles("/onlydir")
		require.NoError(t, err)
		require.Empty(t, files)

		files, err = client.ListFiles("missing-dir")
		require.NoError(t, err)
		require.Empty(t, files)
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
			"onlydir", "onlydir/subdir", "onlydir/subdir/nested.txt",
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
		Hostname:       "localhost:2222",
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
