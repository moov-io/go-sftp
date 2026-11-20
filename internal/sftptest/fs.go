// Copyright 2026 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package sftptest

import (
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
)

// osRoot maps POSIX SFTP paths onto a directory using os.Root.
type osRoot struct {
	root *os.Root
}

func relName(p string) string {
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))
	rel := strings.TrimPrefix(p, "/")
	if rel == "" || rel == "." {
		return "."
	}
	return filepath.FromSlash(rel)
}

func (h *osRoot) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	return h.OpenFile(r)
}

func (h *osRoot) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	return h.OpenFile(r)
}

func (h *osRoot) OpenFile(r *sftp.Request) (sftp.WriterAtReaderAt, error) {
	return h.root.OpenFile(relName(r.Filepath), openFlags(r.Pflags()), 0o644)
}

func openFlags(p sftp.FileOpenFlags) int {
	var flags int
	switch {
	case p.Read && p.Write:
		flags = os.O_RDWR
	case p.Write:
		flags = os.O_WRONLY
	default:
		flags = os.O_RDONLY
	}
	if p.Creat {
		flags |= os.O_CREATE
	}
	if p.Trunc {
		flags |= os.O_TRUNC
	}
	if p.Excl {
		flags |= os.O_EXCL
	}
	return flags
}

func (h *osRoot) Filecmd(r *sftp.Request) error {
	switch r.Method {
	case "Setstat":
		name := relName(r.Filepath)
		attrs := r.Attributes()
		if attrs == nil {
			return os.ErrInvalid
		}
		flags := r.AttrFlags()
		if flags.Permissions {
			if err := h.root.Chmod(name, attrs.FileMode()); err != nil {
				return err
			}
		}
		if flags.Size {
			if attrs.Size > uint64(math.MaxInt64) {
				return os.ErrInvalid
			}
			fd, err := h.root.OpenFile(name, os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			err = fd.Truncate(int64(attrs.Size))
			if closeErr := fd.Close(); err == nil {
				err = closeErr
			}
			return err
		}
		return nil
	case "Rename", "PosixRename":
		return h.root.Rename(relName(r.Filepath), relName(r.Target))
	case "Rmdir":
		return h.root.Remove(relName(r.Filepath))
	case "Remove":
		name := relName(r.Filepath)
		info, err := h.root.Lstat(name)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.ErrInvalid
		}
		return h.root.Remove(name)
	case "Mkdir":
		return h.root.Mkdir(relName(r.Filepath), 0o755)
	default:
		return os.ErrInvalid
	}
}

func (h *osRoot) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	name := relName(r.Filepath)

	switch r.Method {
	case "List":
		fd, err := h.root.Open(name)
		if err != nil {
			return nil, err
		}
		defer fd.Close()
		entries, err := fd.ReadDir(-1)
		if err != nil {
			return nil, err
		}
		infos := make([]os.FileInfo, 0, len(entries))
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			infos = append(infos, info)
		}
		return listerat(infos), nil
	case "Stat":
		info, err := h.root.Stat(name)
		if err != nil {
			return nil, err
		}
		return listerat{info}, nil
	default:
		return nil, os.ErrInvalid
	}
}

func (h *osRoot) Lstat(r *sftp.Request) (sftp.ListerAt, error) {
	info, err := h.root.Lstat(relName(r.Filepath))
	if err != nil {
		return nil, err
	}
	return listerat{info}, nil
}

func (h *osRoot) RealPath(p string) (string, error) {
	if p == "" || p == "." {
		return "/", nil
	}
	p = path.Clean(p)
	if !path.IsAbs(p) {
		p = path.Join("/", p)
	}
	return p, nil
}

type listerat []os.FileInfo

func (f listerat) ListAt(ls []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(f)) {
		return 0, io.EOF
	}
	n := copy(ls, f[offset:])
	if n < len(ls) {
		return n, io.EOF
	}
	return n, nil
}

var (
	_ sftp.OpenFileWriter     = (*osRoot)(nil)
	_ sftp.RealPathFileLister = (*osRoot)(nil)
	_ sftp.LstatFileLister    = (*osRoot)(nil)
)
