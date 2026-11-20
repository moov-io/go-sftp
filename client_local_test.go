package go_sftp_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/moov-io/base/log"
	gosftp "github.com/moov-io/go-sftp"
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
	absOutbox := filepath.ToSlash(filepath.Join(root, "outbox"))
	absArchive := filepath.ToSlash(filepath.Join(root, "outbox", "archive"))

	// Named relative directories walk from Getwd with "[/?]<dir>/*". The local
	// pkg/sftp server's Getwd is the temp root, not "/", so those listings are
	// empty. This is master's behavior, intentionally preserved for compatibility.
	t.Run("relative named dir", func(t *testing.T) {
		files, err := client.ListFiles("outbox")
		require.NoError(t, err)
		require.Empty(t, files)
	})

	t.Run("relative trailing slash", func(t *testing.T) {
		files, err := client.ListFiles("outbox/")
		require.NoError(t, err)
		require.Empty(t, files)
	})

	t.Run("dot slash prefix", func(t *testing.T) {
		files, err := client.ListFiles("./outbox")
		require.NoError(t, err)
		require.Empty(t, files)
	})

	t.Run("cwd empty and dot", func(t *testing.T) {
		files, err := client.ListFiles("")
		require.NoError(t, err)
		require.Equal(t, []string{"root.txt"}, files)

		files, err = client.ListFiles(".")
		require.NoError(t, err)
		require.Equal(t, []string{"root.txt"}, files)
	})

	t.Run("slash lists working directory", func(t *testing.T) {
		files, err := client.ListFiles("/")
		require.NoError(t, err)
		require.Equal(t, []string{"/root.txt"}, files)
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

	t.Run("relative subdir", func(t *testing.T) {
		files, err := client.ListFiles("outbox/archive")
		require.NoError(t, err)
		require.Empty(t, files)
	})

	t.Run("absolute named dir", func(t *testing.T) {
		files, err := client.ListFiles(absOutbox)
		require.NoError(t, err)
		require.Equal(t, []string{absOutbox + "/ppd.ach"}, files)
	})

	t.Run("absolute subdir", func(t *testing.T) {
		files, err := client.ListFiles(absArchive)
		require.NoError(t, err)
		require.Equal(t, []string{absArchive + "/three.txt"}, files)
	})

	t.Run("open known relative path", func(t *testing.T) {
		file, err := client.Open("outbox/ppd.ach")
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

	t.Run("relative named dir is empty when Getwd is not /", func(t *testing.T) {
		files, err := client.ListFiles("outbox/ARCHIVE")
		require.NoError(t, err)
		require.Empty(t, files)
	})

	t.Run("absolute dir matches case-insensitively", func(t *testing.T) {
		dir := filepath.ToSlash(filepath.Join(root, "outbox", "ARCHIVE"))
		files, err := client.ListFiles(dir)
		require.NoError(t, err)
		require.Len(t, files, 1)
		require.Equal(t, "three.txt", path.Base(files[0]))
		require.True(t, strings.EqualFold(files[0], filepath.ToSlash(filepath.Join(root, "outbox", "archive", "three.txt"))))

		file, err := client.Open(files[0])
		require.NoError(t, err)
		body, err := io.ReadAll(file.Contents)
		require.NoError(t, err)
		require.Equal(t, "three", string(body))
		require.NoError(t, file.Close())
	})
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
	require.Empty(t, files, "relative ListFiles is empty when Getwd is not /")

	abs := filepath.ToSlash(filepath.Join(root, "upload"))
	files, err = client.ListFiles(abs)
	require.NoError(t, err)
	require.Equal(t, []string{abs + "/" + path.Base(name)}, files)

	file, err := client.Open(name)
	require.NoError(t, err)
	body, err := io.ReadAll(file.Contents)
	require.NoError(t, err)
	require.Equal(t, "payload", string(body))
	require.NoError(t, file.Close())

	require.NoError(t, client.Delete(name))
	files, err = client.ListFiles(abs)
	require.NoError(t, err)
	require.Empty(t, files)

	// Delete of a missing file returns nil. Intentionally preserved for compatibility.
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
	file, err := client.Open(name)
	require.NoError(t, err)
	body, err := io.ReadAll(file.Contents)
	require.NoError(t, err)
	require.Equal(t, "x", string(body))
	require.NoError(t, file.Close())
}

func TestClientLocal_ReconnectAfterClose(t *testing.T) {
	root := t.TempDir()
	writeLocalTree(t, root, map[string]string{"outbox/ppd.ach": "ppd"})
	client := newLocalClient(t, root, nil)

	require.NoError(t, client.Close())
	file, err := client.Open("outbox/ppd.ach")
	require.NoError(t, err)
	body, err := io.ReadAll(file.Contents)
	require.NoError(t, err)
	require.Equal(t, "ppd", string(body))
	require.NoError(t, file.Close())
}

func TestClientLocal_ListFilePathIsEmpty(t *testing.T) {
	root := t.TempDir()
	writeLocalTree(t, root, map[string]string{"outbox/ppd.ach": "ppd"})
	client := newLocalClient(t, root, nil)

	files, err := client.ListFiles("outbox/ppd.ach")
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestClientLocal_CaseSiblingDirs_PinsMasterCaseInsensitiveUnion(t *testing.T) {
	// Characterization of master's ListFiles: matching is case-insensitive, so
	// an absolute listing of any of these siblings returns the union of all four.
	// Every returned path is a real file, and Open of each same.ach returns the
	// contents from that sibling. This behavior is intentionally preserved for
	// compatibility.
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
	if len(ents) < 4 {
		t.Skip("case-insensitive FS")
	}

	client := newLocalClient(t, root, nil)

	var want []string
	for _, n := range names {
		base := filepath.ToSlash(filepath.Join(root, "x", n))
		want = append(want, base+"/"+n+".ach", base+"/same.ach")
	}

	for _, n := range names {
		dir := filepath.ToSlash(filepath.Join(root, "x", n))
		files, err := client.ListFiles(dir)
		require.NoError(t, err, "ListFiles(%q)", dir)
		require.ElementsMatch(t, want, files, "ListFiles(%q) should return the union of all four case-sibling dirs", dir)

		for _, f := range files {
			file, err := client.Open(f)
			require.NoError(t, err, "Open(%q) every listed path must be real", f)
			body, err := io.ReadAll(file.Contents)
			require.NoError(t, err)
			require.NoError(t, file.Close())
			parent := path.Base(path.Dir(f))
			if path.Base(f) == "same.ach" {
				require.Equal(t, "from-"+parent, string(body), "Open(%q)", f)
			} else {
				require.Equal(t, parent, string(body), "Open(%q)", f)
			}
		}

		rel, err := client.ListFiles("x/" + n)
		require.NoError(t, err)
		require.Empty(t, rel, "relative ListFiles(%q) is empty when Getwd is not /", "x/"+n)
	}
}

func TestClientLocal_ListFilesUnreadableDirFailsAllListings(t *testing.T) {
	// Characterization of master's ListFiles: the glob walk starts at Getwd and
	// visits the entire tree, so one unreadable directory anywhere under Getwd
	// makes every ListFiles call fail. Intentionally preserved for compatibility.
	if runtime.GOOS == "windows" {
		t.Skip("directory mode 000 is not enforced on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores mode 000")
	}

	root := t.TempDir()
	writeLocalTree(t, root, map[string]string{"outbox/ppd.ach": "ppd"})
	locked := filepath.Join(root, "locked")
	require.NoError(t, os.Mkdir(locked, 0o000))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() {
		_ = os.Chmod(locked, 0o755)
	})

	client := newLocalClient(t, root, nil)

	absOutbox := filepath.ToSlash(filepath.Join(root, "outbox"))
	files, err := client.ListFiles(absOutbox)
	require.Error(t, err, "ListFiles(%q)", absOutbox)
	require.ErrorContains(t, err, "permission denied")
	require.Empty(t, files)

	files, err = client.ListFiles(".")
	require.Error(t, err, "ListFiles(%q)", ".")
	require.ErrorContains(t, err, "permission denied")
	require.Empty(t, files)
}

func TestClientLocal_ListFilesRootAndSymlink(t *testing.T) {
	// ListFiles("/") lists the working directory and does not follow symlink
	// directories (Walk uses Lstat). Intentionally preserved for compatibility.
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "real"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "real", "a.ach"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "top.txt"), []byte("t"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "afile"), []byte("t"), 0o644))
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink: %v", err)
	}

	client := newLocalClient(t, root, nil)

	files, err := client.ListFiles("/")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"/afile", "/linked", "/top.txt"}, files)

	files, err = client.ListFiles("linked")
	require.NoError(t, err)
	require.Empty(t, files)

	files, err = client.ListFiles(filepath.ToSlash(filepath.Join(root, "linked")))
	require.NoError(t, err)
	require.Empty(t, files)
}
