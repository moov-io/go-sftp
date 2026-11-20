package go_sftp_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"net"
	"sync/atomic"
	"testing"

	pkgsftp "github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func newHostSigner(t *testing.T) ssh.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	return signer
}

func newClientRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func privateKeyPEM(t *testing.T, key *rsa.PrivateKey, passphrase string) string {
	t.Helper()
	var block *pem.Block
	var err error
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(key, "")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte(passphrase))
	}
	require.NoError(t, err)
	return string(pem.EncodeToMemory(block))
}

func clientPublicKey(t *testing.T, key *rsa.PrivateKey) ssh.PublicKey {
	t.Helper()
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	return signer.PublicKey()
}

func startLocalSFTPServer(t *testing.T, workDir string) (addr, hostKey string) {
	t.Helper()
	return startLocalSFTPServerAuth(t, workDir, nil)
}

func startLocalSFTPServerAuth(t *testing.T, workDir string, clientPub ssh.PublicKey) (addr, hostKey string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	signer := newHostSigner(t)
	serveLocalSFTP(t, listener, signer, workDir, clientPub)
	return listener.Addr().String(), string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
}

func serveLocalSFTP(t *testing.T, listener net.Listener, signer ssh.Signer, workDir string, clientPub ssh.PublicKey) {
	t.Helper()
	serveLocalSFTPWhen(t, listener, signer, workDir, clientPub, nil)
}

func serveLocalSFTPWhen(t *testing.T, listener net.Listener, signer ssh.Signer, workDir string, clientPub ssh.PublicKey, ready *atomic.Bool) {
	t.Helper()

	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "demo" && string(pass) == "password" {
				return nil, nil
			}
			return nil, fmt.Errorf("invalid credentials")
		},
	}
	if clientPub != nil {
		want := clientPub.Marshal()
		config.PublicKeyCallback = func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == "demo" && bytes.Equal(key.Marshal(), want) {
				return nil, nil
			}
			return nil, fmt.Errorf("unknown public key")
		}
	}
	config.AddHostKey(signer)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			nConn, err := listener.Accept()
			if err != nil {
				return
			}
			if ready != nil && !ready.Load() {
				nConn.Close()
				continue
			}
			go handleLocalSFTP(nConn, config, workDir)
		}
	}()

	t.Cleanup(func() {
		listener.Close()
		<-done
	})
}

func handleLocalSFTP(nConn net.Conn, config *ssh.ServerConfig, workDir string) {
	conn, chans, reqs, err := ssh.NewServerConn(nConn, config)
	if err != nil {
		nConn.Close()
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		ch, reqs, err := newChannel.Accept()
		if err != nil {
			return
		}
		go func() {
			defer ch.Close()
			for req := range reqs {
				if req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp" {
					req.Reply(true, nil)
					server, err := pkgsftp.NewServer(ch, pkgsftp.WithServerWorkingDirectory(workDir))
					if err != nil {
						return
					}
					_ = server.Serve()
					return
				}
				if req.WantReply {
					req.Reply(false, nil)
				}
			}
		}()
	}
}
