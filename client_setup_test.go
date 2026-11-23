// Copyright 2026 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package go_sftp_test

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/moov-io/base/log"
	sftp "github.com/moov-io/go-sftp"
	"github.com/moov-io/go-sftp/internal/sftptest"
	"github.com/stretchr/testify/require"
)

func TestTestdataLineEndings(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "outbox", "one.txt"))
	require.NoError(t, err)
	require.Equal(t, "one\n", string(data))
}

func startFixtureServer(t *testing.T) *sftptest.Server {
	t.Helper()

	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, os.DirFS("testdata")))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "upload"), 0o755))
	return sftptest.Start(t, root)
}

func testClient(t *testing.T) sftp.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("-short flag was provided")
	}
	return newClientFromServer(t, startFixtureServer(t))
}

func defaultClientConfig(hostname string, hostKeys []string) *sftp.ClientConfig {
	return &sftp.ClientConfig{
		Hostname:       hostname,
		Username:       sftptest.Username,
		Password:       sftptest.Password,
		Timeout:        5 * time.Second,
		MaxConnections: 1,
		PacketSize:     32000,
		HostPublicKeys: hostKeys,
	}
}

func newClientFromServer(t *testing.T, srv *sftptest.Server) sftp.Client {
	t.Helper()
	return newClientFromConfig(t, defaultClientConfig(srv.Addr, []string{srv.HostPublicKey}))
}

func newClientFromConfig(t *testing.T, cfg *sftp.ClientConfig) sftp.Client {
	t.Helper()
	client, err := sftp.NewClient(log.NewTestLogger(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	return client
}

func waitForOpenSSH(t *testing.T) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:2222", 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func requireOpenSSH(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("-short flag was provided")
	}
	if waitForOpenSSH(t) {
		return
	}
	if os.Getenv("CI") != "" && runtime.GOOS != "windows" {
		t.Fatal("OpenSSH SFTP server not running on 127.0.0.1:2222")
	}
	t.Skip("OpenSSH SFTP server not running on 127.0.0.1:2222")
}

func openSSHClient(t *testing.T) sftp.Client {
	t.Helper()
	requireOpenSSH(t)
	return newClientFromConfig(t, defaultClientConfig("127.0.0.1:2222", nil))
}

func caseInsensitiveFS(t *testing.T, root string) bool {
	t.Helper()
	upper := filepath.Join(root, "outbox", "Upper")
	lower := filepath.Join(root, "outbox", "upper")
	uInfo, err1 := os.Stat(upper)
	lInfo, err2 := os.Stat(lower)
	if err1 != nil || err2 != nil {
		return false
	}
	return os.SameFile(uInfo, lInfo)
}
