package go_sftp_test

import (
	"testing"

	sftp "github.com/moov-io/go-sftp"
	"github.com/stretchr/testify/require"
)

func TestClientConfig_HostKeys(t *testing.T) {
	tests := []struct {
		name string
		cfg  sftp.ClientConfig
		want []string
	}{
		{
			name: "no host keys",
			cfg:  sftp.ClientConfig{},
			want: nil,
		},
		{
			name: "only HostPublicKey",
			cfg: sftp.ClientConfig{
				HostPublicKey: "public-key",
			},
			want: []string{"public-key"},
		},
		{
			name: "only HostPublicKeys",
			cfg: sftp.ClientConfig{
				HostPublicKeys: []string{
					"public-key-1",
					"public-key-2",
				},
			},
			want: []string{"public-key-1", "public-key-2"},
		},
		{
			name: "combined and unique",
			cfg: sftp.ClientConfig{
				HostPublicKey: "public-key",
				HostPublicKeys: []string{
					"public-key",
					"public-key-1",
					"public-key-1",
				},
			},
			want: []string{"public-key", "public-key-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.HostKeys()
			require.Equal(t, tt.want, got)
		})
	}
}
