package go_sftp_test

import (
	"testing"

	sftp "github.com/moov-io/go-sftp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const (
	rsaKey     = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQD1MU4KKe56DW+cnEomhmk0JMp5dS5LUDvrNM8cRE8i/JxPRsEbrHsta7/1Bj6jutAVTvHVSDrCZ5c+TIXlhSGQEfbjlXMiu9vP4vewdFTfm1xUdryv8MO5+Tas0HlbO9h92aV/SBpBxMLCIBVM9U+zKxmskxR1QMQZ7tzRGMnYMhQD74V6ANnwndDAlWspF+LcaUaDQqjeMDTv86q+ki4uDID5dwvx4eX11exfT+LwCvTMpCKhPJawA7QwnXNVvSEu/4p9EkNKr1xNIoiJdIwOnWrX8kAmlVkwL1cKCQF7wOfneYjKxJUMKwKtPZ9qtMmeOlhO7pLxhbtjcwvfIg69"
	ecdsaKey   = "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBEQFGqHGgr0e0jyq2ojt1TJgsFdLrn9w6iYXn1oWvuiOQgVAUL/6vrwQQ7ncbqM7/ZOaonx3C2Kr2IZHIXRmVXc="
	ed25519Key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPZ3WQItO2r2wfGrjedz9LGwlLFgIUM6GbIpBKvaxiSz"

	mismatchKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAINnH6Geq7YNlClxNhCMN0IVt1f0XsPyMYqlW5htNYLpy"
)

func TestMultiKeyCallback_Check(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "host key mismatch",
			key:     mismatchKey,
			wantErr: true,
		},
		{
			name:    "rsa match",
			key:     rsaKey,
			wantErr: false,
		},
		{
			name:    "ecdsa match",
			key:     "example.io " + ecdsaKey,
			wantErr: false,
		},
		{
			name:    "ed25519 match",
			key:     ed25519Key,
			wantErr: false,
		},
	}

	callback, err := sftp.NewMultiKeyCallback([]string{
		rsaKey,
		ecdsaKey,
		ed25519Key,
	})
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(tt.key))
			require.NoError(t, err)

			err = callback("", nil, hostKey)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
