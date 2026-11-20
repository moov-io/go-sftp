package go_sftp

import (
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListRequiresWorkingDirectoryJoin(t *testing.T) {
	require.False(t, listRequiresWorkingDirectoryJoin(""))
	require.False(t, listRequiresWorkingDirectoryJoin("."))
	require.False(t, listRequiresWorkingDirectoryJoin("./"))
	require.False(t, listRequiresWorkingDirectoryJoin("/"))
	require.False(t, listRequiresWorkingDirectoryJoin("/outbox"))
	require.False(t, listRequiresWorkingDirectoryJoin("/outbox/"))
	require.True(t, listRequiresWorkingDirectoryJoin("outbox"))
	require.True(t, listRequiresWorkingDirectoryJoin("outbox/"))
	require.True(t, listRequiresWorkingDirectoryJoin("./outbox"))
	require.True(t, listRequiresWorkingDirectoryJoin("./Watchlist Review/"))
}

func TestWorkingDirectoryRequiresJoin(t *testing.T) {
	require.False(t, workingDirectoryRequiresJoin(""))
	require.False(t, workingDirectoryRequiresJoin("."))
	require.False(t, workingDirectoryRequiresJoin("/"))
	require.False(t, workingDirectoryRequiresJoin("/."))
	require.True(t, workingDirectoryRequiresJoin("/home/user"))
	require.True(t, workingDirectoryRequiresJoin("/home/user/"))
}

func TestEscapeGlob(t *testing.T) {
	pattern := "[/?]" + escapeGlob("home/user[1]/outbox") + "/*"
	ok, err := path.Match(strings.ToLower(pattern), strings.ToLower("/home/user[1]/outbox/ppd.ach"))
	require.NoError(t, err)
	require.True(t, ok)

	unescaped := "[/?]home/user[1]/outbox/*"
	ok, err = path.Match(strings.ToLower(unescaped), strings.ToLower("/home/user[1]/outbox/ppd.ach"))
	require.NoError(t, err)
	require.False(t, ok)
}

func TestListFilesPatternIsPOSIX(t *testing.T) {
	cleaned := path.Clean(strings.TrimPrefix("/home/user/outbox", "/"))
	pattern := "[/?]" + escapeGlob(cleaned) + "/*"
	ok, err := path.Match(strings.ToLower(pattern), "/home/user/outbox/ppd.ach")
	require.NoError(t, err)
	require.True(t, ok)
}

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
			"/home/user",
			"/home/user/outbox",
			"outbox",
		)
		require.Equal(t, []string{"outbox/ppd.ach"}, got)
	})

	t.Run("trailing slash input", func(t *testing.T) {
		got := rebaseListResults(
			[]string{"/outbox/one.txt"},
			"/",
			"/outbox",
			"outbox/",
		)
		require.Equal(t, []string{"outbox/one.txt"}, got)
	})

	t.Run("empty dir", func(t *testing.T) {
		got := rebaseListResults([]string{"/root.txt"}, "/", "/", "")
		require.Equal(t, []string{"root.txt"}, got)
	})

	t.Run("dot dir", func(t *testing.T) {
		got := rebaseListResults([]string{"/root.txt"}, "/", "/", ".")
		require.Equal(t, []string{"root.txt"}, got)
	})

	t.Run("absolute input unchanged when search matches", func(t *testing.T) {
		files := []string{"/outbox/one.txt"}
		got := rebaseListResults(files, "/", "/outbox", "/outbox")
		require.Equal(t, files, got)
	})

	t.Run("case-insensitive input keeps server case", func(t *testing.T) {
		got := rebaseListResults(
			[]string{"/outbox/archive/three.txt"},
			"/",
			"/outbox/ARCHIVE",
			"outbox/ARCHIVE",
		)
		require.Equal(t, []string{"outbox/archive/three.txt"}, got)
	})

	t.Run("non-chrooted working directory", func(t *testing.T) {
		got := rebaseListResults(
			[]string{"/home/user/outbox/Upper/names.txt"},
			"/home/user",
			"/home/user/outbox/upper",
			"outbox/upper",
		)
		require.Equal(t, []string{"outbox/Upper/names.txt"}, got)
	})

	t.Run("root listing stays absolute", func(t *testing.T) {
		got := rebaseListResults([]string{"/root.txt"}, "/", "/", "/")
		require.Equal(t, []string{"/root.txt"}, got)
	})
}
