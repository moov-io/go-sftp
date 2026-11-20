// Copyright 2026 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package sftptest

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const (
	Username = "demo"
	Password = "password"
)

// Server is an in-process SSH+SFTP server listening on 127.0.0.1.
type Server struct {
	Addr          string
	HostPublicKey string
	Root          string
}

type serveOpts struct {
	handlers  sftp.Handlers
	workDir   string
	clientPub ssh.PublicKey
	ready     *atomic.Bool
	signer    ssh.Signer
	listener  net.Listener
}

// Start serves the OS directory at root as a chrooted SFTP filesystem.
// Remote path / maps to root. The host filesystem's case rules apply, so
// Windows CI exercises a case-insensitive server.
func Start(t testing.TB, root string) *Server {
	t.Helper()

	fs, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	t.Cleanup(func() { _ = fs.Close() })

	h := &osRoot{root: fs}
	srv := serve(t, serveOpts{
		handlers: sftp.Handlers{
			FileGet:  h,
			FilePut:  h,
			FileCmd:  h,
			FileList: h,
		},
	})
	srv.Root = root
	return srv
}

// StartInMemory serves an empty in-memory filesystem. Paths are
// case-sensitive on every OS, including Windows.
func StartInMemory(t testing.TB) *Server {
	t.Helper()
	return serve(t, serveOpts{handlers: sftp.InMemHandler()})
}

// StartWorkDir serves workDir with pkg/sftp.NewServer so Getwd is workDir,
// not "/". Named relative ListFiles is empty on that layout.
func StartWorkDir(t testing.TB, workDir string) *Server {
	t.Helper()
	srv := serve(t, serveOpts{workDir: workDir})
	srv.Root = workDir
	return srv
}

// StartWorkDirAuth is StartWorkDir plus public-key authentication for clientPub.
func StartWorkDirAuth(t testing.TB, workDir string, clientPub ssh.PublicKey) *Server {
	t.Helper()
	srv := serve(t, serveOpts{workDir: workDir, clientPub: clientPub})
	srv.Root = workDir
	return srv
}

// ServeWorkDir serves workDir on an existing listener. If ready is non-nil,
// connections are closed until ready is true.
func ServeWorkDir(t testing.TB, ln net.Listener, signer ssh.Signer, workDir string, clientPub ssh.PublicKey, ready *atomic.Bool) *Server {
	t.Helper()
	srv := serve(t, serveOpts{
		workDir:   workDir,
		clientPub: clientPub,
		ready:     ready,
		signer:    signer,
		listener:  ln,
	})
	srv.Root = workDir
	return srv
}

func serve(t testing.TB, opts serveOpts) *Server {
	t.Helper()

	signer := opts.signer
	if signer == nil {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate host key: %v", err)
		}
		signer, err = ssh.NewSignerFromKey(priv)
		if err != nil {
			t.Fatalf("host key signer: %v", err)
		}
	}

	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == Username && string(pass) == Password {
				return nil, nil
			}
			return nil, fmt.Errorf("rejected")
		},
	}
	if opts.clientPub != nil {
		want := opts.clientPub.Marshal()
		config.PublicKeyCallback = func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == Username && bytes.Equal(key.Marshal(), want) {
				return nil, nil
			}
			return nil, fmt.Errorf("unknown public key")
		}
	}
	config.AddHostKey(signer)

	ln := opts.listener
	if ln == nil {
		var err error
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
	}

	var mu sync.Mutex
	var closed bool
	var conns []net.Conn
	done := make(chan struct{})

	srv := &Server{
		Addr:          ln.Addr().String(),
		HostPublicKey: string(ssh.MarshalAuthorizedKey(signer.PublicKey())),
	}

	t.Cleanup(func() {
		mu.Lock()
		closed = true
		mu.Unlock()
		ln.Close()
		<-done
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	})

	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			if opts.ready != nil && !opts.ready.Load() {
				conn.Close()
				continue
			}
			mu.Lock()
			if closed {
				mu.Unlock()
				conn.Close()
				return
			}
			conns = append(conns, conn)
			mu.Unlock()
			go handleSSH(conn, config, opts.handlers, opts.workDir)
		}
	}()

	return srv
}

func handleSSH(nConn net.Conn, config *ssh.ServerConfig, handlers sftp.Handlers, workDir string) {
	defer nConn.Close()

	sconn, chans, reqs, err := ssh.NewServerConn(nConn, config)
	if err != nil {
		return
	}
	defer sconn.Close()

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			return
		}
		go serveSFTP(channel, requests, handlers, workDir)
	}
}

func serveSFTP(channel ssh.Channel, requests <-chan *ssh.Request, handlers sftp.Handlers, workDir string) {
	defer channel.Close()

	for req := range requests {
		ok := req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp"
		_ = req.Reply(ok, nil)
		if !ok {
			continue
		}
		if workDir != "" {
			server, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(workDir))
			if err != nil {
				return
			}
			_ = server.Serve()
			_ = server.Close()
			return
		}
		server := sftp.NewRequestServer(channel, handlers)
		_ = server.Serve()
		_ = server.Close()
		return
	}
}
