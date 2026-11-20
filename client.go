// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

// Package go_sftp provides an SFTP client for uploading, listing, and opening
// files on a remote server.
//
// The package name is go_sftp, so import it with an alias:
//
//	import gosftp "github.com/moov-io/go-sftp"
//
// NewClient accepts a moov-io/base logger (a nil logger is allowed) and a
// ClientConfig. If the initial connection fails, NewClient still returns a
// non-nil Client along with the error so later calls can retry. Set at least
// one of HostPublicKey or HostPublicKeys; leaving both empty disables host-key
// verification.
package go_sftp

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/moov-io/base/log"
	"github.com/pkg/sftp"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ssh"
)

var (
	sftpAgentUp = prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
		Name: "sftp_agent_up",
		Help: "Status of SFTP agent connection",
	}, []string{"hostname"})

	sftpConnectionRetries = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Name: "sftp_connection_retries",
		Help: "Counter of SFTP connection retry attempts",
	}, []string{"hostname"})
)

type Client interface {
	Ping() error
	Close() error

	Open(path string) (*File, error)
	Reader(path string) (*File, error)

	Delete(path string) error
	UploadFile(path string, contents io.ReadCloser) error

	ListFiles(dir string) ([]string, error)
	Walk(dir string, fn fs.WalkDirFunc) error
}

type client struct {
	logger log.Logger
	cfg    ClientConfig

	mu     sync.Mutex // protects all read/write methods
	conn   *ssh.Client
	client *sftp.Client
}

// NewClient returns an SFTP client for cfg.
//
// If the first connection attempt fails, NewClient still returns a non-nil
// Client along with the error so later calls can retry. If cfg is nil, the
// Client is nil.
//
// MaxConnections defaults to 1 when it is zero or negative. A nil logger is
// allowed; pass a logger if you want connection and host-key warnings recorded
// through moov-io/base. When host-key checking is skipped and logger is nil,
// the warning is written to standard error.
func NewClient(logger log.Logger, cfg *ClientConfig) (Client, error) {
	if cfg == nil {
		return nil, errors.New("nil SFTP config")
	}

	cc := &client{cfg: *cfg, logger: logger}
	if cc.cfg.MaxConnections <= 0 {
		cc.cfg.MaxConnections = 1
	}

	conn, err := cc.connection()
	cc.record(err) // track up metric for remote server
	err = cc.clearConnectionOnError(err)

	// Print an initial startup message
	if conn != nil && logger != nil {
		wd, wdErr := conn.Getwd()
		if wdErr != nil {
			err = cc.clearConnectionOnError(wdErr)
		}
		if wd != "" {
			logger.Logf("starting SFTP client in %s", wd)
		}
	}

	return cc, err
}

// connection returns an sftp.Client which is connected to the remote server.
// This function will attempt to establish a new connection if none exists already.
//
// connection must be called within a mutex lock.
func (c *client) connection() (*sftp.Client, error) {
	if c == nil {
		return nil, errors.New("nil client / config")
	}

	if c.client != nil {
		// Verify the connection works and if not drop through and reconnect
		if _, err := c.client.Getwd(); err == nil {
			return c.client, nil
		} else {
			// Our connection is having issues, so retry connecting
			c.client.Close()
		}
	}

	conn, stdin, stdout, err := sftpConnect(c.logger, c.cfg)
	if err != nil {
		return nil, fmt.Errorf("sftp: %w", err)
	}
	c.conn = conn

	// Setup our SFTP client
	var opts = []sftp.ClientOption{
		sftp.MaxConcurrentRequestsPerFile(c.cfg.MaxConnections),
	}
	if c.cfg.PacketSize > 0 {
		opts = append(opts, sftp.MaxPacket(c.cfg.PacketSize))
	}

	// client, err := sftp.NewClient(conn, opts...)
	client, err := sftp.NewClientPipe(stdout, stdin, opts...)
	if err != nil {
		if conn != nil {
			go conn.Close()
		}
		return nil, fmt.Errorf("sftp: sftp connect: %w", err)
	}
	c.client = client

	return c.client, nil
}

// clearConnectionOnError accepts any error from a call involving the SSH/SFTP connection.
// If an error is encountered that causes either connection (SSH or SFTP) to be
// lost it will tear down the connections. The next invocation of c.connection()
// will re-establish new connections.
//
// When the error is captured by clearConnectionOnError the client will attempt to reconnect
// and that new connection error will be returned.
func (c *client) clearConnectionOnError(err error) error {
	if err == nil {
		return nil
	}
	// Possible errors from github.com/pkg/sftp/request-errors.go
	switch {
	case errors.Is(err, sftp.ErrSSHFxEOF),
		errors.Is(err, sftp.ErrSSHFxFailure),
		errors.Is(err, sftp.ErrSSHFxBadMessage),
		errors.Is(err, sftp.ErrSSHFxNoConnection),
		errors.Is(err, sftp.ErrSSHFxConnectionLost):
		// Teardown the existing connections
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		if c.client != nil {
			c.client.Close()
			c.client = nil
		}
		// Reconnect if needed and replace the initial error
		_, err = c.connection()
	}
	return err
}

var (
	hostKeyCallbackOnce sync.Once
	hostKeyCallback     = func(logger log.Logger) {
		msg := "sftp: WARNING!!! Insecure default of skipping SFTP host key validation. Please set HostPublicKey(s)"
		if logger != nil {
			logger.Warn().Log(msg)
		} else {
			fmt.Fprintln(os.Stderr, msg)
		}
	}
)

func sftpConnect(logger log.Logger, cfg ClientConfig) (*ssh.Client, io.WriteCloser, io.Reader, error) {
	conf := &ssh.ClientConfig{
		User:    cfg.Username,
		Timeout: cfg.Timeout,
	}
	conf.SetDefaults()

	if hostKeys := cfg.HostKeys(); len(hostKeys) > 0 {
		callback, err := NewMultiKeyCallback(hostKeys)
		if err != nil {
			return nil, nil, nil, err
		}
		conf.HostKeyCallback = callback
	} else {
		hostKeyCallbackOnce.Do(func() {
			hostKeyCallback(logger)
		})
		//nolint:gosec
		conf.HostKeyCallback = ssh.InsecureIgnoreHostKey() // insecure default
	}
	// Setup various Authentication methods
	if cfg.Password != "" {
		conf.Auth = append(conf.Auth, ssh.Password(cfg.Password))
	}
	if cfg.ClientPrivateKey != "" {
		signer, err := readSigner(cfg.ClientPrivateKey, cfg.ClientPrivateKeyPassword)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("sftpConnect: failed to read client private key: %w", err)
		}
		conf.Auth = append(conf.Auth, ssh.PublicKeys(signer))
	}

	// Connect to the remote server
	var client *ssh.Client
	var err error
	for i := 0; i < 3; i++ {
		if client == nil {
			if i > 0 {
				sftpConnectionRetries.With("hostname", cfg.Hostname).Add(1)
			}
			client, err = ssh.Dial("tcp", cfg.Hostname, conf) // retry connection
			time.Sleep(250 * time.Millisecond)
		}
	}
	if client == nil {
		if err != nil {
			return nil, nil, nil, fmt.Errorf("sftpConnect: %w", err)
		}
		return nil, nil, nil, fmt.Errorf("sftpConnect: unable to establish ssh connection")
	}

	session, err := client.NewSession()
	if err != nil {
		if client != nil {
			go client.Close()
		}
		return nil, nil, nil, err
	}
	if err = session.RequestSubsystem("sftp"); err != nil {
		if client != nil {
			go client.Close()
		}
		return nil, nil, nil, err
	}
	pw, err := session.StdinPipe()
	if err != nil {
		if client != nil {
			go client.Close()
		}
		return nil, nil, nil, err
	}
	pr, err := session.StdoutPipe()
	if err != nil {
		if client != nil {
			go client.Close()
		}
		return nil, nil, nil, err
	}

	return client, pw, pr, nil
}

func readSigner(raw, passphrase string) (ssh.Signer, error) {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if len(decoded) > 0 && err == nil {
		return readPrivateKey(decoded, passphrase)
	}
	return readPrivateKey([]byte(raw), passphrase)
}

func readPrivateKey(data []byte, passphrase string) (ssh.Signer, error) {
	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
	}
	return ssh.ParsePrivateKey(data)
}

func (c *client) Ping() error {
	if c == nil {
		return errors.New("nil SFTPTransferAgent")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := c.connection()
	c.record(err)
	err = c.clearConnectionOnError(err)
	if err != nil {
		return err
	}

	_, err = conn.ReadDir(".")
	c.record(err)
	err = c.clearConnectionOnError(err)
	if err != nil {
		return fmt.Errorf("sftp: ping %w", err)
	}
	return nil
}

func (c *client) record(err error) {
	if c == nil {
		return
	}
	if err != nil {
		sftpAgentUp.With("hostname", c.cfg.Hostname).Set(0)
	} else {
		sftpAgentUp.With("hostname", c.cfg.Hostname).Set(1)
	}
}

func (c *client) Close() error {
	if c == nil {
		return nil
	}
	if c.client != nil {
		c.client.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
	return nil
}

// Delete removes the file at path. If the file does not exist, Delete returns nil.
func (c *client) Delete(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := c.connection()
	err = c.clearConnectionOnError(err)
	if err != nil {
		return err
	}

	info, err := conn.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // file doesn't exist
		}

		// The error is something else related to STAT so return that
		err = c.clearConnectionOnError(err)
		return fmt.Errorf("sftp: delete stat: %w", err)
	}

	if info != nil {
		err := conn.Remove(path)
		err = c.clearConnectionOnError(err)
		if err != nil {
			return fmt.Errorf("sftp: delete: %w", err)
		}
	}

	return nil // not found
}

// UploadFile creates a file containing the provided contents at the specified path
//
// The File's contents will always be closed
func (c *client) UploadFile(path string, contents io.ReadCloser) error {
	defer contents.Close()

	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := c.connection()
	err = c.clearConnectionOnError(err)
	if err != nil {
		return err
	}

	// Create the directory if it doesn't exist
	if !c.cfg.SkipDirectoryCreation {
		dir, _ := filepath.Split(path)

		info, err := conn.Stat(dir)
		err = c.clearConnectionOnError(err)
		if info == nil || err != nil {
			if os.IsNotExist(err) || strings.Contains(err.Error(), "file does not exist") {
				err := conn.MkdirAll(dir)
				err = c.clearConnectionOnError(err)
				if err != nil {
					return fmt.Errorf("sftp: problem creating %s as parent dir: %w", dir, err)
				}
			} else {
				return fmt.Errorf("problem checking if %s exists: %w", dir, err)
			}
		}
	}

	// Some servers don't allow you to open a file for reading and writing at the same time.
	// For these we follow the pkg/sftp docs to open files for writing (not reading).
	fd, err := conn.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	err = c.clearConnectionOnError(err)
	if err != nil {
		return fmt.Errorf("sftp: problem creating remote file %s: %w", path, err)
	}
	if fd == nil {
		return fmt.Errorf("sftp: nil fd opening: %s", path)
	}

	n, err := io.Copy(fd, contents)
	if err != nil {
		return fmt.Errorf("sftp: problem copying (n=%d) %s: %w", n, path, err)
	}

	if !c.cfg.SkipSyncAfterUpload {
		err = fd.Sync()
		err = c.clearConnectionOnError(err)
		if err != nil {
			// Skip sync if the remote server doesn't support it
			if !strings.Contains(err.Error(), "SSH_FX_OP_UNSUPPORTED") {
				return fmt.Errorf("sftp: problem with sync on %s: %v", path, err)
			}
		}
	}

	if !c.cfg.SkipChmodAfterUpload {
		err := fd.Chmod(0600)
		err = c.clearConnectionOnError(err)
		if err != nil {
			return fmt.Errorf("sftp: problem chmod %s: %w", path, err)
		}
	}

	err = fd.Close()
	err = c.clearConnectionOnError(err)
	if err != nil {
		return fmt.Errorf("sftp: closing %s after writing failed: %w", path, err)
	}

	return nil
}

// ListFiles returns the files in dir, not including files in subdirectories.
// Use Walk to traverse a tree recursively.
//
// Paths are returned in the same style as dir: an absolute dir yields absolute
// paths, and a relative dir yields relative paths. ListFiles tries ReadDir
// first (the server resolves relative paths against its working directory).
// A successful ReadDir, including an empty directory, is returned as-is so a
// listing that already works never changes spelling. If ReadDir fails because
// the path is missing (including case mismatch), ListFiles falls back to a
// case-insensitive glob walk, then joins a named relative dir with Getwd only
// when that walk is also empty and the working directory is not "/".
//
// Paths are matched in case-insensitive comparisons, but results are returned exactly as they
// appear on the server.
func (c *client) ListFiles(dir string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	filenames, err := c.readDirFiles(dir)
	if err == nil {
		return filenames, nil
	}
	if !isNotFound(err) && !isNotDir(err) {
		return nil, fmt.Errorf("listing %s failed: %w", dir, err)
	}

	filenames, err = c.listFilesAt(dir)
	if err != nil {
		return nil, fmt.Errorf("listing %s failed: %w", dir, err)
	}
	if len(filenames) > 0 || !listRequiresWorkingDirectoryJoin(dir) {
		return filenames, nil
	}

	conn, err := c.connection()
	if err = c.clearConnectionOnError(err); err != nil {
		return filenames, nil
	}
	wd, err := conn.Getwd()
	if err = c.clearConnectionOnError(err); err != nil {
		return filenames, nil
	}
	if !workingDirectoryRequiresJoin(wd) {
		return filenames, nil
	}

	searchDir := resolveListDirectory(wd, dir)
	if searchDir == "" || searchDir == dir {
		return filenames, nil
	}
	joined, err := c.listFilesAt(searchDir)
	if err != nil || len(joined) == 0 {
		return filenames, nil
	}
	return rebaseListResults(joined, wd, searchDir, dir), nil
}

func listRequiresWorkingDirectoryJoin(dir string) bool {
	switch path.Clean(dir) {
	case ".", "/":
		return false
	}
	return !strings.HasPrefix(dir, "/")
}

func workingDirectoryRequiresJoin(wd string) bool {
	switch path.Clean(wd) {
	case "", ".", "/":
		return false
	}
	return true
}

func (c *client) readDirFiles(dir string) ([]string, error) {
	conn, err := c.connection()
	if err = c.clearConnectionOnError(err); err != nil {
		return nil, err
	}

	listDir := dir
	switch dir {
	case "", ".", "./":
		listDir = "."
	}

	infos, err := conn.ReadDir(listDir)
	if err != nil {
		return nil, err
	}

	prefix := dir
	switch dir {
	case "", ".", "./", "/":
	default:
		prefix = canonicalDir(conn, dir)
	}

	var filenames []string
	for _, info := range infos {
		if info == nil || info.IsDir() {
			continue
		}
		name := info.Name()
		var out string
		switch dir {
		case "", ".", "./":
			out = name
		case "/":
			out = "/" + name
		default:
			out = path.Join(prefix, name)
		}
		if strings.HasPrefix(dir, "/") && !strings.HasPrefix(out, "/") {
			out = "/" + out
		}
		filenames = append(filenames, out)
	}
	return filenames, nil
}

// canonicalDir rewrites dir using the names the server reports for each
// component, so a case-insensitive ReadDir of "/outbox/upper" still returns
// "/outbox/Upper/..." when that is the on-disk spelling.
func canonicalDir(conn *sftp.Client, dir string) string {
	leading := strings.HasPrefix(dir, "/")
	trim := strings.Trim(path.Clean(dir), "/")
	if trim == "" || trim == "." {
		return dir
	}
	parts := strings.Split(trim, "/")
	parent := "."
	if leading {
		parent = "/"
	}
	var rebuilt []string
	for _, part := range parts {
		actual := part
		infos, err := conn.ReadDir(parent)
		if err == nil {
			for _, info := range infos {
				if info != nil && info.IsDir() && strings.EqualFold(info.Name(), part) {
					actual = info.Name()
					break
				}
			}
		}
		rebuilt = append(rebuilt, actual)
		if parent == "/" {
			parent = "/" + actual
		} else if parent == "." {
			parent = actual
		} else {
			parent = path.Join(parent, actual)
		}
	}
	out := path.Join(rebuilt...)
	if leading && !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	return out
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist) || errors.Is(err, sftp.ErrSSHFxNoSuchFile) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "file does not exist") || strings.Contains(msg, "no such file")
}

func isNotDir(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not a directory") || strings.Contains(msg, "not a dir")
}

// resolveListDirectory joins a relative dir with the SFTP working directory
// so listings work on servers whose working directory is not "/".
func resolveListDirectory(wd, dir string) string {
	switch dir {
	case "", ".", "./":
		if wd == "" || wd == "." {
			return dir
		}
		return wd
	}
	if strings.HasPrefix(dir, "/") {
		return dir
	}
	if wd == "" || wd == "." {
		return dir
	}
	return path.Join(wd, dir)
}

// rebaseListResults rewrites paths found under searchDir back into the style
// of the directory the caller passed to ListFiles. Absolute inputs are returned
// unchanged. Relative inputs are returned relative to the working directory wd,
// keeping the exact case the server reported.
func rebaseListResults(files []string, wd, searchDir, inputDir string) []string {
	if len(files) == 0 || searchDir == inputDir || strings.HasPrefix(inputDir, "/") {
		return files
	}

	prefix := strings.TrimSuffix(wd, "/") + "/"
	out := make([]string, 0, len(files))
	for _, file := range files {
		if len(file) >= len(prefix) && strings.EqualFold(file[:len(prefix)], prefix) {
			file = file[len(prefix):]
		}
		out = append(out, file)
	}
	return out
}

func (c *client) listFilesAt(dir string) ([]string, error) {
	cleaned := path.Clean(strings.TrimPrefix(dir, "/"))
	pattern := cleaned
	indexPrefix := ""

	wd := "."
	switch {
	case dir == "/":
		pattern = "*"
	case cleaned == ".":
		if dir == "" {
			pattern = "*"
		} else {
			pattern = path.Join(dir, "*")
		}
	case cleaned != "":
		indexPrefix = cleaned + "/"
		pattern = "[/?]" + escapeGlob(cleaned) + "/*"
		conn, err := c.connection()
		if err = c.clearConnectionOnError(err); err != nil {
			return nil, err
		}
		wd, err = conn.Getwd()
		if err = c.clearConnectionOnError(err); err != nil {
			return nil, err
		}
	}

	var filenames []string
	err := c.walkNoLock(wd, func(remotePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Check if the server's path matches what we're searching for in a case-insensitive comparison.
		matches, err := path.Match(strings.ToLower(pattern), strings.ToLower(remotePath))
		if matches && err == nil {
			out := remotePath
			if indexPrefix != "" {
				idx := strings.Index(strings.ToLower(remotePath), strings.ToLower(indexPrefix))
				if idx > -1 {
					out = remotePath[idx:]
				} else {
					out = path.Join(dir, path.Base(remotePath))
				}
			}
			if strings.HasPrefix(dir, "/") && !strings.HasPrefix(out, "/") {
				out = "/" + out
			}
			filenames = append(filenames, out)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return filenames, nil
}

func escapeGlob(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '*', '?', '[', '\\':
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Reader will open the file at path and provide a reader to access its contents.
// Callers need to close the returned Contents
//
// Callers should be aware that network errors while reading can occur since contents
// are streamed from the SFTP server.
func (c *client) Reader(path string) (*File, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := c.connection()
	err = c.clearConnectionOnError(err)
	if err != nil {
		return nil, err
	}

	fd, err := conn.Open(path)
	err = c.clearConnectionOnError(err)
	if err != nil {
		return nil, fmt.Errorf("sftp: open %s: %w", path, err)
	}
	if fd == nil {
		return nil, fmt.Errorf("sftp: nil fd after opening %s", path)
	}

	var fileinfo fs.FileInfo
	modTime := time.Now().In(time.UTC)
	if stat, _ := fd.Stat(); stat != nil {
		fileinfo = stat
		modTime = stat.ModTime()
	}

	return &File{
		Filename: fd.Name(),
		Contents: fd,
		ModTime:  modTime,
		fileinfo: fileinfo,
	}, nil
}

// Open will return the contents at path and consume the entire file contents.
// WARNING: This method can use a lot of memory by consuming the entire file into memory.
func (c *client) Open(path string) (*File, error) {
	r, err := c.Reader(path)
	if err != nil {
		return nil, err
	}

	// read the entire remote file
	var buf bytes.Buffer
	if n, err := io.Copy(&buf, r.Contents); err != nil {
		r.Close()
		if err != nil && !strings.Contains(err.Error(), sftp.ErrInternalInconsistency.Error()) {
			return nil, fmt.Errorf("sftp: read (n=%d) %s: %w", n, r.Filename, err)
		}
		return nil, fmt.Errorf("sftp: read (n=%d) on %s: %w", n, r.Filename, err)
	} else {
		r.Close()
	}

	return &File{
		Filename: r.Filename,
		Contents: io.NopCloser(&buf),
		ModTime:  r.ModTime,
	}, nil
}

// Walk will traverse dir and call fs.WalkDirFunc on each entry.
//
// Follow the docs for fs.WalkDirFunc for details on traversal. Walk accepts fs.SkipDir to not process directories.
func (c *client) Walk(dir string, fn fs.WalkDirFunc) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.walkNoLock(dir, fn)
}

func (c *client) walkNoLock(dir string, fn fs.WalkDirFunc) error {
	conn, err := c.connection()
	if err = c.clearConnectionOnError(err); err != nil {
		return err
	}

	w := conn.Walk(dir)
	if w == nil {
		return errors.New("nil *fs.Walker")
	}

	var skippedDirs []string

	// Pass the callback to each file found
	for w.Step() {
		if err := w.Err(); err != nil {
			return err
		}

		var skip bool
		for _, sd := range skippedDirs {
			if isWithinDir(w.Path(), sd) {
				skip = true
				break
			}
		}
		if skip {
			w.SkipDir()
			continue
		}

		info := w.Stat()
		if info == nil {
			continue
		}

		err := fn(w.Path(), fs.FileInfoToDirEntry(info), w.Err())
		if err != nil {
			if err == fs.SkipDir {
				// SkipDir on a directory skips that directory. On a file it skips
				// the remaining entries of the directory containing the file.
				skipped := w.Path()
				if !info.IsDir() {
					skipped = path.Dir(skipped)
				}
				skippedDirs = append(skippedDirs, skipped)

				w.SkipDir()
			} else {
				return err
			}
		}
	}
	return w.Err()
}

// isWithinDir reports whether p is dir itself or an entry below it.
// Remote paths always use forward slashes.
func isWithinDir(p, dir string) bool {
	if p == dir {
		return true
	}
	if dir == "." {
		return !strings.HasPrefix(p, "/")
	}
	return strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}
