package go_sftp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveListDirectory(t *testing.T) {
	tests := []struct {
		wd, dir, want string
	}{
		{wd: "/home/user", dir: "outbox", want: "/home/user/outbox"},
		{wd: "/home/user", dir: "outbox/", want: "/home/user/outbox"},
		{wd: "/home/user", dir: "./outbox", want: "/home/user/outbox"},
		{wd: "/home/user", dir: "/abs/outbox", want: "/abs/outbox"},
		{wd: "/home/user", dir: "", want: "/home/user"},
		{wd: "/home/user", dir: ".", want: "/home/user"},
		{wd: "/", dir: "outbox", want: "/outbox"},
		{wd: "/", dir: "/outbox", want: "/outbox"},
		{wd: "/", dir: "", want: "/"},
		{wd: "/", dir: ".", want: "/"},
		{wd: "/", dir: "/", want: "/"},
		{wd: ".", dir: "outbox", want: "outbox"},
	}
	for _, tt := range tests {
		got := resolveListDirectory(tt.wd, tt.dir)
		require.Equal(t, tt.want, got, "wd=%q dir=%q", tt.wd, tt.dir)
	}
}

func TestRebaseListResults(t *testing.T) {
	t.Run("relative input from absolute search", func(t *testing.T) {
		got := rebaseListResults(
			[]string{"/home/user/outbox/ppd.ach"},
			"/home/user/outbox",
			"outbox",
		)
		require.Equal(t, []string{"outbox/ppd.ach"}, got)
	})

	t.Run("trailing slash input", func(t *testing.T) {
		got := rebaseListResults(
			[]string{"/outbox/one.txt"},
			"/outbox",
			"outbox/",
		)
		require.Equal(t, []string{"outbox/one.txt"}, got)
	})

	t.Run("empty dir", func(t *testing.T) {
		got := rebaseListResults([]string{"/root.txt"}, "/", "")
		require.Equal(t, []string{"root.txt"}, got)
	})

	t.Run("dot dir", func(t *testing.T) {
		got := rebaseListResults([]string{"/root.txt"}, "/", ".")
		require.Equal(t, []string{"root.txt"}, got)
	})

	t.Run("absolute input unchanged when search matches", func(t *testing.T) {
		files := []string{"/outbox/one.txt"}
		got := rebaseListResults(files, "/outbox", "/outbox")
		require.Equal(t, files, got)
	})

	t.Run("root listing stays absolute", func(t *testing.T) {
		got := rebaseListResults([]string{"/root.txt"}, "/", "/")
		require.Equal(t, []string{"/root.txt"}, got)
	})
}
