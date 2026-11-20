package go_sftp

import (
	"errors"
	"os"
	"testing"

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
