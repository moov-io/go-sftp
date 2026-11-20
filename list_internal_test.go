package go_sftp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListFilesInMemory(t *testing.T) {
	// The in-memory RequestServer's Getwd is "/", so relative named dirs still
	// match the "[/?]<dir>/*" glob. This pins master's listing on a chroot-like
	// working directory.
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
