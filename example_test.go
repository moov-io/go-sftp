package go_sftp_test

import (
	"fmt"

	gosftp "github.com/moov-io/go-sftp"
)

func ExampleClientConfig() {
	cfg := &gosftp.ClientConfig{
		Hostname:       "sftp.example.com:22",
		Username:       "alice",
		Password:       "correct-horse",
		HostPublicKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleHostKey"},
	}

	// Import the package as gosftp because the package name is go_sftp.
	// NewClient accepts a moov-io/base logger (nil is allowed).
	// MaxConnections defaults to 1 when left at zero.
	fmt.Println(cfg.Hostname)
	// Output:
	// sftp.example.com:22
}
