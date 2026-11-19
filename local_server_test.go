package go_sftp_test

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moov-io/base/log"
	gosftp "github.com/moov-io/go-sftp"
	pkgsftp "github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestListFilesRelativeWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "outbox"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "outbox", "ppd.ach"), []byte("ppd"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "root.txt"), []byte("root"), 0o644))

	addr, hostKey := startLocalSFTPServer(t, root)

	client, err := gosftp.NewClient(log.NewTestLogger(), &gosftp.ClientConfig{
		Hostname:       addr,
		Username:       "demo",
		Password:       "password",
		Timeout:        5 * time.Second,
		MaxConnections: 0, // should default to 1
		HostPublicKeys: []string{hostKey},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	files, err := client.ListFiles("outbox")
	require.NoError(t, err)
	require.Equal(t, []string{"outbox/ppd.ach"}, files)

	files, err = client.ListFiles("outbox/")
	require.NoError(t, err)
	require.Equal(t, []string{"outbox/ppd.ach"}, files)

	files, err = client.ListFiles("./outbox")
	require.NoError(t, err)
	require.Equal(t, []string{"outbox/ppd.ach"}, files)

	files, err = client.ListFiles("")
	require.NoError(t, err)
	require.Equal(t, []string{"root.txt"}, files)

	file, err := client.Open("outbox/ppd.ach")
	require.NoError(t, err)
	require.Equal(t, "outbox/ppd.ach", file.Filename)
	require.False(t, file.ModTime.IsZero())
	body, err := io.ReadAll(file.Contents)
	require.NoError(t, err)
	require.Equal(t, "ppd", string(body))
	require.NoError(t, file.Close())
}

func startLocalSFTPServer(t *testing.T, workDir string) (addr, hostKey string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)

	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "demo" && string(pass) == "password" {
				return nil, nil
			}
			return nil, fmt.Errorf("invalid credentials")
		},
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
			go handleLocalSFTP(nConn, config, workDir)
		}
	}()

	t.Cleanup(func() {
		listener.Close()
		<-done
	})

	return listener.Addr().String(), string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
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
