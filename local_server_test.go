// Copyright 2026 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"net"
	"sync/atomic"
	"testing"

	"github.com/moov-io/go-sftp/internal/sftptest"
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
	srv := sftptest.StartWorkDir(t, workDir)
	return srv.Addr, srv.HostPublicKey
}

func startLocalSFTPServerAuth(t *testing.T, workDir string, clientPub ssh.PublicKey) (addr, hostKey string) {
	t.Helper()
	srv := sftptest.StartWorkDirAuth(t, workDir, clientPub)
	return srv.Addr, srv.HostPublicKey
}

func serveLocalSFTPWhen(t *testing.T, listener net.Listener, signer ssh.Signer, workDir string, clientPub ssh.PublicKey, ready *atomic.Bool) {
	t.Helper()
	sftptest.ServeWorkDir(t, listener, signer, workDir, clientPub, ready)
}
