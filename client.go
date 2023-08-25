// Copyright 2022 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/moov-io/base/log"
	"github.com/moov-io/go-sftp/pkg/sshx"

	"github.com/go-kit/kit/metrics/prometheus"
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

type ClientConfig struct {
	Hostname string
	Username string
	Password string

	Timeout        time.Duration
	MaxConnections int
	PacketSize     int

	HostPublicKey string

	// ClientPrivateKey must be a base64 encoded string
	ClientPrivateKey         string
	ClientPrivateKeyPassword string // not base64 encoded

	SkipChmodAfterUpload  bool
	SkipDirectoryCreation bool
}

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
	conn   *ssh.Client
	client *sftp.Client
	cfg    ClientConfig
	logger log.Logger
	mu     sync.Mutex // protects all read/write methods
}

func NewClient(logger log.Logger, cfg *ClientConfig) (Client, error) {
	if cfg == nil {
		return nil, errors.New("nil SFTP config")
	}

	cc := &client{cfg: *cfg, logger: logger}

	conn, err := cc.connection()
	cc.record(err) // track up metric for remote server

	// Print an initial startup message
	if conn != nil && logger != nil {
		wd, _ := conn.Getwd()
		logger.Logf("starting SFTP client in %s", wd)
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
		go conn.Close()
		return nil, fmt.Errorf("sftp: sftp connect: %w", err)
	}
	c.client = client

	return c.client, nil
}

var (
	hostKeyCallbackOnce sync.Once
	hostKeyCallback     = func(logger log.Logger) {
		msg := "sftp: WARNING!!! Insecure default of skipping SFTP host key validation. Please set sftp_configs.host_public_key"
		if logger != nil {
			logger.Warn().Log(msg)
		} else {
			fmt.Println(msg)
		}
	}
)

func sftpConnect(logger log.Logger, cfg ClientConfig) (*ssh.Client, io.WriteCloser, io.Reader, error) {
	conf := &ssh.ClientConfig{
		User:    cfg.Username,
		Timeout: cfg.Timeout,
	}
	conf.SetDefaults()

	if cfg.HostPublicKey != "" {
		pubKey, err := sshx.ReadPubKey([]byte(cfg.HostPublicKey))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("problem parsing ssh public key: %w", err)
		}
		conf.HostKeyCallback = ssh.FixedHostKey(pubKey)
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
	if client == nil && err != nil {
		return nil, nil, nil, fmt.Errorf("sftpConnect: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		go client.Close()
		return nil, nil, nil, err
	}
	if err = session.RequestSubsystem("sftp"); err != nil {
		go client.Close()
		return nil, nil, nil, err
	}
	pw, err := session.StdinPipe()
	if err != nil {
		go client.Close()
		return nil, nil, nil, err
	}
	pr, err := session.StdoutPipe()
	if err != nil {
		go client.Close()
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
	if err != nil {
		return err
	}

	_, err = conn.ReadDir(".")
	c.record(err)
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

func (c *client) Delete(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := c.connection()
	if err != nil {
		return err
	}

	info, err := conn.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("sftp: delete stat: %w", err)
	}
	if info != nil {
		if err := conn.Remove(path); err != nil {
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
	if err != nil {
		return err
	}

	// Create the directory if it doesn't exist
	if !c.cfg.SkipDirectoryCreation {
		dir, _ := filepath.Split(path)

		info, err := conn.Stat(dir)
		if info == nil || err != nil {
			if os.IsNotExist(err) || strings.Contains(err.Error(), "file does not exist") {
				if err := conn.MkdirAll(dir); err != nil {
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
	if err != nil {
		return fmt.Errorf("sftp: problem creating remote file %s: %w", path, err)
	}

	n, err := io.Copy(fd, contents)
	if err != nil {
		return fmt.Errorf("sftp: problem copying (n=%d) %s: %w", n, path, err)
	}

	if err := fd.Sync(); err != nil {
		// Skip sync if the remote server doesn't support it
		if !strings.Contains(err.Error(), "SSH_FX_OP_UNSUPPORTED") {
			return fmt.Errorf("sftp: problem with sync on %s: %v", path, err)
		}
	}

	if !c.cfg.SkipChmodAfterUpload {
		if err = fd.Chmod(0600); err != nil {
			return fmt.Errorf("sftp: problem chmod %s: %w", path, err)
		}
	}

	if err := fd.Close(); err != nil {
		return fmt.Errorf("sftp: closing %s after writing failed: %w", path, err)
	}

	return nil
}

// ListFiles will return the paths of files within dir. Paths are returned as locations from dir,
// so if dir is an absolute path the returned paths will be.
//
// Paths are matched in case-insensitive comparisons, but results are returned exactly as they
// appear on the server.
func (c *client) ListFiles(dir string) ([]string, error) {
	pattern := filepath.Clean(strings.TrimPrefix(dir, string(os.PathSeparator)))
	wd := "."
	var err error
	switch {
	case dir == "/":
		pattern = "*"
	case pattern == ".":
		if dir == "" {
			pattern = "*"
		} else {
			pattern = filepath.Join(dir, "*")
		}
	case pattern != "":
		pattern = "[/?]" + pattern + "/*"
		wd, err = c.client.Getwd()
		if err != nil {
			return nil, err
		}
	}

	var filenames []string
	err = c.Walk(wd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Check if the server's path matches what we're searching for in a case-insensitive comparison.
		matches, err := filepath.Match(strings.ToLower(pattern), strings.ToLower(path))
		if matches && err == nil {
			// Return the path with exactly the case on the server.
			trimmedPattern := strings.TrimPrefix(strings.TrimSuffix(pattern, "*"), "[/?]")
			idx := strings.Index(strings.ToLower(path), strings.ToLower(trimmedPattern))
			if idx > -1 {
				path = path[idx:]
				if strings.HasPrefix(dir, "/") && !strings.HasPrefix(path, "/") {
					path = "/" + path
				}
				filenames = append(filenames, path)
			} else {
				// Fallback to Go logic of presenting the path
				filenames = append(filenames, filepath.Join(dir, filepath.Base(path)))
			}
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("listing %s failed: %w", dir, err)
	}
	return filenames, nil
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
	if err != nil {
		return nil, err
	}

	fd, err := conn.Open(path)
	if err != nil {
		return nil, fmt.Errorf("sftp: open %s: %w", path, err)
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

	w := c.client.Walk(dir)
	if w == nil {
		return errors.New("nil *fs.Walker")
	}
	// Pass the callback to each file found
	for w.Step() {
		info := w.Stat()
		if info.IsDir() {
			continue
		}
		err := fn(w.Path(), fs.FileInfoToDirEntry(info), w.Err())
		if err != nil {
			if err == fs.SkipDir {
				w.SkipDir()
			} else {
				return err
			}
		}
	}
	return w.Err()
}
