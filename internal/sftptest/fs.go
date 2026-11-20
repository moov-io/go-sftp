// Copyright 2026 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package sftptest

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
)

// osRoot maps POSIX SFTP paths onto a directory on the host filesystem.
type osRoot struct {
	root string
}

func (h *osRoot) translate(p string) (string, error) {
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))
	rel := strings.TrimPrefix(p, "/")

	absRoot, err := filepath.Abs(h.root)
	if err != nil {
		return "", err
	}

	var local string
	if rel == "" || rel == "." {
		local = absRoot
	} else {
		local = filepath.Join(absRoot, filepath.FromSlash(rel))
	}

	absLocal, err := filepath.Abs(local)
	if err != nil {
		return "", err
	}
	relToRoot, err := filepath.Rel(absRoot, absLocal)
	if err != nil {
		return "", err
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) {
		return "", os.ErrPermission
	}
	return absLocal, nil
}

func (h *osRoot) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	return h.OpenFile(r)
}

func (h *osRoot) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	return h.OpenFile(r)
}

func (h *osRoot) OpenFile(r *sftp.Request) (sftp.WriterAtReaderAt, error) {
	local, err := h.translate(r.Filepath)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(local, openFlags(r.Pflags()), 0o644)
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
		local, err := h.translate(r.Filepath)
		if err != nil {
			return err
		}
		attrs := r.Attributes()
		if attrs == nil {
			return os.ErrInvalid
		}
		flags := r.AttrFlags()
		if flags.Permissions {
			if err := os.Chmod(local, attrs.FileMode()); err != nil {
				return err
			}
		}
		if flags.Size {
			if err := os.Truncate(local, int64(attrs.Size)); err != nil {
				return err
			}
		}
		return nil
	case "Rename", "PosixRename":
		src, err := h.translate(r.Filepath)
		if err != nil {
			return err
		}
		dst, err := h.translate(r.Target)
		if err != nil {
			return err
		}
		return os.Rename(src, dst)
	case "Rmdir":
		local, err := h.translate(r.Filepath)
		if err != nil {
			return err
		}
		return os.Remove(local)
	case "Remove":
		local, err := h.translate(r.Filepath)
		if err != nil {
			return err
		}
		info, err := os.Lstat(local)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.ErrInvalid
		}
		return os.Remove(local)
	case "Mkdir":
		local, err := h.translate(r.Filepath)
		if err != nil {
			return err
		}
		return os.Mkdir(local, 0o755)
	default:
		return os.ErrInvalid
	}
}

func (h *osRoot) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	local, err := h.translate(r.Filepath)
	if err != nil {
		return nil, err
	}

	switch r.Method {
	case "List":
		entries, err := os.ReadDir(local)
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
		info, err := os.Stat(local)
		if err != nil {
			return nil, err
		}
		return listerat{info}, nil
	default:
		return nil, os.ErrInvalid
	}
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
)
