// Copyright 2026 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package sftptest

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"sync"
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

// Start serves the OS directory at root as a chrooted SFTP filesystem.
// Remote path / maps to root. The host filesystem's case rules apply, so
// Windows CI exercises a case-insensitive server.
func Start(t testing.TB, root string) *Server {
	t.Helper()

	h := &osRoot{root: root}
	srv := listenAndServe(t, sftp.Handlers{
		FileGet:  h,
		FilePut:  h,
		FileCmd:  h,
		FileList: h,
	})
	srv.Root = root
	return srv
}

// StartInMemory serves an empty in-memory filesystem. Paths are
// case-sensitive on every OS, including Windows.
func StartInMemory(t testing.TB) *Server {
	t.Helper()
	return listenAndServe(t, sftp.InMemHandler())
}

func listenAndServe(t testing.TB, handlers sftp.Handlers) *Server {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("host key signer: %v", err)
	}

	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == Username && string(pass) == Password {
				return nil, nil
			}
			return nil, fmt.Errorf("rejected")
		},
	}
	config.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var mu sync.Mutex
	var closed bool
	var conns []net.Conn

	srv := &Server{
		Addr:          ln.Addr().String(),
		HostPublicKey: string(ssh.MarshalAuthorizedKey(signer.PublicKey())),
	}

	t.Cleanup(func() {
		mu.Lock()
		closed = true
		mu.Unlock()
		ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			if closed {
				mu.Unlock()
				conn.Close()
				return
			}
			conns = append(conns, conn)
			mu.Unlock()
			go handleSSH(conn, config, handlers)
		}
	}()

	return srv
}

func handleSSH(nConn net.Conn, config *ssh.ServerConfig, handlers sftp.Handlers) {
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
		go func(in <-chan *ssh.Request) {
			for req := range in {
				ok := req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp"
				_ = req.Reply(ok, nil)
			}
		}(requests)

		server := sftp.NewRequestServer(channel, handlers)
		_ = server.Serve()
		server.Close()
		channel.Close()
	}
}
