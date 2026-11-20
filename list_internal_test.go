package go_sftp

import (
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
)

func TestListFilesInMemory(t *testing.T) {
	c := newInMemoryClient(t,
		"/root.txt",
		"/outbox/one.txt",
		"/outbox/two.txt",
		"/outbox/archive/three.txt",
	)

	t.Run("absolute dir skips subdirs", func(t *testing.T) {
		files, err := c.ListFiles("/outbox")
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"/outbox/one.txt", "/outbox/two.txt"}, files)
	})

	t.Run("relative dir", func(t *testing.T) {
		files, err := c.ListFiles("outbox")
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"outbox/one.txt", "outbox/two.txt"}, files)
	})

	t.Run("cwd", func(t *testing.T) {
		files, err := c.ListFiles(".")
		require.NoError(t, err)
		require.Equal(t, []string{"root.txt"}, files)
	})

	t.Run("missing dir is empty", func(t *testing.T) {
		files, err := c.ListFiles("/missing")
		require.NoError(t, err)
		require.Empty(t, files)
	})

	t.Run("nested", func(t *testing.T) {
		files, err := c.ListFiles("/outbox/archive")
		require.NoError(t, err)
		require.Equal(t, []string{"/outbox/archive/three.txt"}, files)
	})
}

func TestIsNotFound(t *testing.T) {
	require.False(t, isNotFound(nil))
	require.True(t, isNotFound(os.ErrNotExist))
	require.True(t, isNotFound(sftp.ErrSSHFxNoSuchFile))
	require.True(t, isNotFound(errors.New("sftp: file does not exist")))
	require.False(t, isNotFound(errors.New("permission denied")))
}

func TestIsNotDir(t *testing.T) {
	require.False(t, isNotDir(nil))
	require.True(t, isNotDir(errors.New("not a directory")))
	require.False(t, isNotDir(errors.New("file does not exist")))
}

func TestMatchDirName(t *testing.T) {
	infos := []os.FileInfo{
		fakeDir("Inbound"),
		fakeDir("inbound"),
		fakeDir("INBOUND"),
		fakeFile("readme.txt"),
	}
	require.Equal(t, "inbound", matchDirName("inbound", infos))
	require.Equal(t, "Inbound", matchDirName("Inbound", infos))
	require.Equal(t, "INBOUND", matchDirName("INBOUND", infos))
	require.Equal(t, "inBound", matchDirName("inBound", infos), "ambiguous EqualFold keeps the caller")

	require.Equal(t, "Upper", matchDirName("upper", []os.FileInfo{fakeDir("Upper")}))
	require.Equal(t, "Upper", matchDirName("Upper", []os.FileInfo{fakeDir("Upper")}))
	require.Equal(t, "missing", matchDirName("missing", infos))
}

func TestIsSessionDead(t *testing.T) {
	require.False(t, isSessionDead(nil))
	require.False(t, isSessionDead(sftp.ErrSSHFxFailure))
	require.False(t, isSessionDead(sftp.ErrSSHFxNoSuchFile))
	require.True(t, isSessionDead(io.EOF))
	require.True(t, isSessionDead(sftp.ErrSSHFxConnectionLost))
	require.True(t, isSessionDead(sftp.ErrSSHFxNoConnection))
	require.True(t, isSessionDead(errors.New("use of closed network connection")))
}

func TestShouldFallbackFromReadDir(t *testing.T) {
	require.False(t, shouldFallbackFromReadDir(nil))
	require.False(t, shouldFallbackFromReadDir(sftp.ErrSSHFxConnectionLost))
	require.False(t, shouldFallbackFromReadDir(errors.New("sftp: sftpConnect: dial tcp 127.0.0.1:1: connect: connection refused")))
	require.True(t, shouldFallbackFromReadDir(sftp.ErrSSHFxFailure))
	require.True(t, shouldFallbackFromReadDir(sftp.ErrSSHFxNoSuchFile))
	require.True(t, shouldFallbackFromReadDir(errors.New("not a directory")))
}

type fakeFileInfo struct {
	name string
	dir  bool
}

func fakeDir(name string) os.FileInfo  { return fakeFileInfo{name: name, dir: true} }
func fakeFile(name string) os.FileInfo { return fakeFileInfo{name: name} }

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.dir }
func (f fakeFileInfo) Sys() any           { return nil }
