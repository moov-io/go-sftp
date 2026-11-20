package go_sftp_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moov-io/base/log"
	gosftp "github.com/moov-io/go-sftp"
	pkgsftp "github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
)

func writeLocalTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		fp := filepath.Join(root, filepath.FromSlash(p))
		require.NoError(t, os.MkdirAll(filepath.Dir(fp), 0o755))
		require.NoError(t, os.WriteFile(fp, []byte(body), 0o644))
	}
}

func newLocalClient(t *testing.T, root string, mutate func(*gosftp.ClientConfig)) gosftp.Client {
	t.Helper()
	addr, hostKey := startLocalSFTPServer(t, root)
	cfg := &gosftp.ClientConfig{
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
	client, err := gosftp.NewClient(log.NewTestLogger(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client
}

func TestClientLocal_ListFilesLayouts(t *testing.T) {
	root := t.TempDir()
	writeLocalTree(t, root, map[string]string{
		"root.txt":                 "root",
		"outbox/ppd.ach":           "ppd",
		"outbox/archive/three.txt": "three",
		"inbox/incoming.ach":       "in",
	})
	require.NoError(t, os.MkdirAll(filepath.Join(root, "empty"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "onlydir", "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "onlydir", "subdir", "nested.txt"), []byte("n"), 0o644))

	client := newLocalClient(t, root, nil)

	t.Run("relative named dir", func(t *testing.T) {
		files, err := client.ListFiles("outbox")
		require.NoError(t, err)
		require.Equal(t, []string{"outbox/ppd.ach"}, files)
	})

	t.Run("relative trailing slash", func(t *testing.T) {
		files, err := client.ListFiles("outbox/")
		require.NoError(t, err)
		require.Equal(t, []string{"outbox/ppd.ach"}, files)
	})

	t.Run("dot slash prefix", func(t *testing.T) {
		files, err := client.ListFiles("./outbox")
		require.NoError(t, err)
		require.Equal(t, []string{"outbox/ppd.ach"}, files)
	})

	t.Run("cwd empty and dot", func(t *testing.T) {
		files, err := client.ListFiles("")
		require.NoError(t, err)
		require.Equal(t, []string{"root.txt"}, files)

		files, err = client.ListFiles(".")
		require.NoError(t, err)
		require.Equal(t, []string{"root.txt"}, files)
	})

	t.Run("empty directory", func(t *testing.T) {
		files, err := client.ListFiles("empty")
		require.NoError(t, err)
		require.Empty(t, files)
	})

	t.Run("directory with only subdirs", func(t *testing.T) {
		files, err := client.ListFiles("onlydir")
		require.NoError(t, err)
		require.Empty(t, files)
	})

	t.Run("missing directory", func(t *testing.T) {
		files, err := client.ListFiles("does-not-exist")
		require.NoError(t, err)
		require.Empty(t, files)
	})

	t.Run("subdir files only", func(t *testing.T) {
		files, err := client.ListFiles("outbox/archive")
		require.NoError(t, err)
		require.Equal(t, []string{"outbox/archive/three.txt"}, files)
	})

	t.Run("open listed relative path", func(t *testing.T) {
		files, err := client.ListFiles("outbox")
		require.NoError(t, err)
		require.Len(t, files, 1)
		file, err := client.Open(files[0])
		require.NoError(t, err)
		body, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "ppd", string(body))
		require.NoError(t, file.Close())
	})
}

func TestClientLocal_CaseInsensitiveList(t *testing.T) {
	root := t.TempDir()
	writeLocalTree(t, root, map[string]string{
		"outbox/archive/three.txt": "three",
	})
	client := newLocalClient(t, root, nil)

	files, err := client.ListFiles("outbox/ARCHIVE")
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "three.txt", path.Base(files[0]))
	require.True(t, strings.EqualFold(files[0], "outbox/archive/three.txt"))

	file, err := client.Open(files[0])
	require.NoError(t, err)
	body, err := io.ReadAll(file.Contents)
	require.NoError(t, err)
	require.Equal(t, "three", string(body))
	require.NoError(t, file.Close())
}

func TestClientLocal_UploadListOpenDelete(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "upload"), 0o755))
	client := newLocalClient(t, root, nil)

	name := fmt.Sprintf("upload/%d.txt", time.Now().UnixNano())
	err := client.UploadFile(name, io.NopCloser(bytes.NewBufferString("payload")))
	require.NoError(t, err)

	files, err := client.ListFiles("upload")
	require.NoError(t, err)
	require.Equal(t, []string{name}, files)

	file, err := client.Open(name)
	require.NoError(t, err)
	body, err := io.ReadAll(file.Contents)
	require.NoError(t, err)
	require.Equal(t, "payload", string(body))
	require.NoError(t, file.Close())

	require.NoError(t, client.Delete(name))
	files, err = client.ListFiles("upload")
	require.NoError(t, err)
	require.Empty(t, files)

	require.NoError(t, client.Delete(name))
	require.NoError(t, client.Delete("upload/missing-dir/missing.txt"))
}

func TestClientLocal_SkipChmodAfterUpload(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "upload"), 0o755))
	client := newLocalClient(t, root, func(cfg *gosftp.ClientConfig) {
		cfg.SkipChmodAfterUpload = true
		cfg.SkipSyncAfterUpload = true
	})

	name := "upload/skip-chmod.txt"
	require.NoError(t, client.UploadFile(name, io.NopCloser(bytes.NewBufferString("x"))))
	files, err := client.ListFiles("upload")
	require.NoError(t, err)
	require.Equal(t, []string{name}, files)
}

func TestClientLocal_ReconnectAfterClose(t *testing.T) {
	root := t.TempDir()
	writeLocalTree(t, root, map[string]string{"outbox/ppd.ach": "ppd"})
	client := newLocalClient(t, root, nil)

	require.NoError(t, client.Close())
	files, err := client.ListFiles("outbox")
	require.NoError(t, err)
	require.Equal(t, []string{"outbox/ppd.ach"}, files)
}

func TestClientLocal_ListFilePathIsEmpty(t *testing.T) {
	root := t.TempDir()
	writeLocalTree(t, root, map[string]string{"outbox/ppd.ach": "ppd"})
	client := newLocalClient(t, root, nil)

	files, err := client.ListFiles("outbox/ppd.ach")
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestClientLocal_CaseSiblingDirs(t *testing.T) {
	root := t.TempDir()
	names := []string{"Inbound", "inbound", "INBOUND", "inBound"}
	for _, n := range names {
		dir := filepath.Join(root, "x", n)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, n+".ach"), []byte(n), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "same.ach"), []byte("from-"+n), 0o644))
	}
	ents, err := os.ReadDir(filepath.Join(root, "x"))
	require.NoError(t, err)
	if len(ents) < 2 {
		t.Skip("case-insensitive FS")
	}

	client := newLocalClient(t, root, nil)
	for _, n := range names {
		for _, dir := range []string{filepath.ToSlash(filepath.Join(root, "x", n)), "x/" + n} {
			files, err := client.ListFiles(dir)
			require.NoError(t, err, "ListFiles(%q)", dir)
			require.Len(t, files, 2, "ListFiles(%q) = %v", dir, files)
			for _, f := range files {
				require.Equal(t, n, path.Base(path.Dir(f)), "ListFiles(%q) returned %q", dir, f)
			}

			var same string
			for _, f := range files {
				if path.Base(f) == "same.ach" {
					same = f
				}
			}
			require.NotEmpty(t, same, "ListFiles(%q) missing same.ach in %v", dir, files)

			file, err := client.Open(same)
			require.NoError(t, err, "Open(%q)", same)
			body, err := io.ReadAll(file.Contents)
			require.NoError(t, err)
			require.NoError(t, file.Close())
			require.Equal(t, "from-"+n, string(body), "Open(%q) after ListFiles(%q)", same, dir)
		}
	}
}

func TestClientLocal_ListFilesRootAndSymlink(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "real"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "real", "a.ach"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "top.txt"), []byte("t"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "afile"), []byte("t"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linked")))

	client := newLocalClient(t, root, nil)

	files, err := client.ListFiles("/")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"/afile", "/linked", "/top.txt"}, files)

	files, err = client.ListFiles("linked")
	require.NoError(t, err)
	require.Equal(t, []string{"linked/a.ach"}, files)

	file, err := client.Open(files[0])
	require.NoError(t, err)
	body, err := io.ReadAll(file.Contents)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, "a", string(body))
}

func TestClientLocal_ReadDirFailureFallsBackToWalk(t *testing.T) {
	handlers := pkgsftp.InMemHandler()
	handlers.FileList = wrapFailureList(handlers.FileList)
	addr, hostKey := startLocalRequestServer(t, handlers)

	client, err := gosftp.NewClient(log.NewTestLogger(), &gosftp.ClientConfig{
		Hostname:       addr,
		Username:       "demo",
		Password:       "password",
		Timeout:        5 * time.Second,
		MaxConnections: 1,
		HostPublicKeys: []string{hostKey},
	})
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	require.NoError(t, client.UploadFile("outbox/ppd.ach", io.NopCloser(strings.NewReader("ppd"))))

	files, err := client.ListFiles("outbox")
	require.NoError(t, err)
	require.Equal(t, []string{"outbox/ppd.ach"}, files)

	files, err = client.ListFiles("does-not-exist")
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestClientLocal_GetwdJoinPropagatesError(t *testing.T) {
	const startDir = "/data"
	handlers := pkgsftp.InMemHandler()
	handlers.FileList = wrapJoinFailList(handlers.FileList, startDir)
	addr, hostKey := startLocalRequestServer(t, handlers, pkgsftp.WithStartDirectory(startDir))

	client, err := gosftp.NewClient(log.NewTestLogger(), &gosftp.ClientConfig{
		Hostname:       addr,
		Username:       "demo",
		Password:       "password",
		Timeout:        5 * time.Second,
		MaxConnections: 1,
		HostPublicKeys: []string{hostKey},
	})
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	require.NoError(t, client.UploadFile("/data/keep.txt", io.NopCloser(strings.NewReader("keep"))))

	files, err := client.ListFiles("does-not-exist")
	require.Error(t, err)
	require.Empty(t, files)
}
