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
	"io/ioutil"
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

	ClientPrivateKey         string
	ClientPrivateKeyPassword string
}

type Client interface {
	Ping() error
	Close() error

	Open(path string) (*File, error)
	Delete(path string) error
	UploadFile(path string, contents io.ReadCloser) error

	ListFiles(dir string) ([]string, error)
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
	if cc != nil {
		cc.record(err)
	}

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
		return nil, fmt.Errorf("sftp: %v", err)
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
		return nil, fmt.Errorf("sftp: sftp connect: %v", err)
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
			return nil, nil, nil, fmt.Errorf("problem parsing ssh public key: %v", err)
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
			return nil, nil, nil, fmt.Errorf("sftpConnect: failed to read client private key: %v", err)
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
		return nil, nil, nil, fmt.Errorf("sftpConnect: %v", err)
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
		return fmt.Errorf("sftp: ping %v", err)
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
		return fmt.Errorf("sftp: delete stat: %v", err)
	}
	if info != nil {
		if err := conn.Remove(path); err != nil {
			return fmt.Errorf("sftp: delete: %v", err)
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

	dir, _ := filepath.Split(path)

	// Create the directory if it doesn't exist
	info, err := conn.Stat(dir)
	if info == nil || (err != nil && os.IsNotExist(err)) {
		if err := conn.Mkdir(dir); err != nil {
			return fmt.Errorf("sftp: problem creating parent dir %s: %v", path, err)
		}
	}

	fd, err := conn.Create(path)
	if err != nil {
		return fmt.Errorf("sftp: problem creating %s: %v", path, err)
	}
	defer fd.Close()

	n, err := io.Copy(fd, contents)
	if err != nil {
		return fmt.Errorf("sftp: problem copying (n=%d) %s: %v", n, path, err)
	}

	if err := fd.Chmod(0600); err != nil {
		return fmt.Errorf("sftp: problem chmod %s: %v", path, err)
	}

	return nil
}

func (c *client) ListFiles(dir string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := c.connection()
	if err != nil {
		return nil, err
	}

	infos, err := conn.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("sftp: readdir %s: %v", dir, err)
	}

	c.logger.Logf("found %d files: %#v", len(infos), infos)

	var filenames []string
	for _, info := range infos {
		if info.IsDir() {
			continue
		}

		filenames = append(filenames, filepath.Join(dir, info.Name()))
	}
	return filenames, nil
}

func (c *client) Open(path string) (*File, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := c.connection()
	if err != nil {
		return nil, err
	}

	fd, err := conn.Open(path)
	if err != nil {
		return nil, fmt.Errorf("sftp: open %s: %v", path, err)
	}

	// download the remote file to our local directory
	var buf bytes.Buffer
	if n, err := io.Copy(&buf, fd); err != nil {
		fd.Close()
		if err != nil && !strings.Contains(err.Error(), sftp.ErrInternalInconsistency.Error()) {
			return nil, fmt.Errorf("sftp: read (n=%d) %s: %v", n, fd.Name(), err)
		}
		return nil, fmt.Errorf("sftp: read (n=%d) on %s: %v", n, fd.Name(), err)
	} else {
		fd.Close()
	}

	return &File{
		Filename: fd.Name(),
		Contents: ioutil.NopCloser(&buf),
	}, nil
}
