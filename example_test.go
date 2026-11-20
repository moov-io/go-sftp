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
		MaxConnections: 1,
	}

	// Import the package as gosftp because the package name is go_sftp.
	// NewClient accepts a moov-io/base logger (nil is allowed).
	// MaxConnections must be >= 1; zero makes NewClient return a pkg/sftp error after SSH connects.
	fmt.Println(cfg.Hostname)
	// Output:
	// sftp.example.com:22
}
