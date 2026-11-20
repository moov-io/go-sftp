// Copyright 2026 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package sftptest

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestStart_RoundTrip(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hi\n"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(root, "dir"), 0o755))

	srv := Start(t, root)
	client := dial(t, srv)

	wd, err := client.Getwd()
	require.NoError(t, err)
	require.Equal(t, "/", wd)

	fd, err := client.Open("/hello.txt")
	require.NoError(t, err)
	body, err := io.ReadAll(fd)
	require.NoError(t, err)
	require.Equal(t, "hi\n", string(body))
	require.NoError(t, fd.Close())

	require.NoError(t, client.Mkdir("/dir/nested"))
	require.NoError(t, writeFile(client, "/dir/nested/out.txt", "out"))

	got, err := client.ReadDir("/")
	require.NoError(t, err)
	var names []string
	for _, info := range got {
		names = append(names, info.Name())
	}
	require.ElementsMatch(t, []string{"hello.txt", "dir"}, names)
}

func TestStartInMemory_CaseSensitive(t *testing.T) {
	srv := StartInMemory(t)
	client := dial(t, srv)

	require.NoError(t, writeFile(client, "/Alpha.txt", "A"))
	require.NoError(t, writeFile(client, "/alpha.txt", "a"))

	fd, err := client.Open("/Alpha.txt")
	require.NoError(t, err)
	body, err := io.ReadAll(fd)
	require.NoError(t, err)
	require.Equal(t, "A", string(body))
	require.NoError(t, fd.Close())

	_, err = client.Open("/ALPHA.txt")
	require.Error(t, err)
}

func TestStart_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "inside.txt"), []byte("in"), 0o644))
	secret := filepath.Join(filepath.Dir(root), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("secret"), 0o644))
	t.Cleanup(func() { os.Remove(secret) })

	srv := Start(t, root)
	client := dial(t, srv)

	// POSIX clean of /../secret.txt is /secret.txt, which is inside the jail.
	_, err := client.Open("/../secret.txt")
	require.Error(t, err)

	fd, err := client.Open("/inside.txt")
	require.NoError(t, err)
	require.NoError(t, fd.Close())
}

func TestOsRoot_RejectsEscape(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "inside.txt"), []byte("in"), 0o644))
	fs, err := os.OpenRoot(root)
	require.NoError(t, err)
	t.Cleanup(func() { _ = fs.Close() })
	h := &osRoot{root: fs}

	fd, err := h.root.Open(relName("/inside.txt"))
	require.NoError(t, err)
	require.NoError(t, fd.Close())

	if runtime.GOOS != "windows" {
		_, err = h.root.Open("..")
		require.Error(t, err)
		return
	}
	_, err = h.root.Open(relName("/C:/Windows/win.ini"))
	require.Error(t, err)
}

func TestStartWorkDir_Getwd(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "root.txt"), []byte("ok"), 0o644))
	srv := StartWorkDir(t, root)
	client := dial(t, srv)

	wd, err := client.Getwd()
	require.NoError(t, err)
	want := filepath.ToSlash(filepath.Clean(root))
	if runtime.GOOS == "windows" {
		want = "/" + want
	}
	require.Equal(t, want, filepath.ToSlash(wd))
}

func TestStart_LstatDoesNotFollowSymlinkDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "real"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "real", "a.txt"), []byte("a"), 0o644))
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink: %v", err)
	}

	srv := Start(t, root)
	client := dial(t, srv)

	w := client.Walk("/")
	var paths []string
	for w.Step() {
		require.NoError(t, w.Err())
		paths = append(paths, w.Path())
	}
	require.NoError(t, w.Err())
	require.Contains(t, paths, "/linked")
	require.NotContains(t, paths, "/linked/a.txt")
	require.Contains(t, paths, "/real/a.txt")
}

func dial(t *testing.T, srv *Server) *sftp.Client {
	t.Helper()

	hostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(srv.HostPublicKey))
	require.NoError(t, err)

	conn, err := ssh.Dial("tcp", srv.Addr, &ssh.ClientConfig{
		User:            Username,
		Auth:            []ssh.AuthMethod{ssh.Password(Password)},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
	})
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	client, err := sftp.NewClient(conn)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	return client
}

func writeFile(client *sftp.Client, name, contents string) error {
	fd, err := client.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	defer fd.Close()
	_, err = io.Copy(fd, strings.NewReader(contents))
	return err
}
