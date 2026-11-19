package go_sftp

import "time"

type ClientConfig struct {
	// Hostname is the remote address, including port (for example "sftp.example.com:22").
	Hostname string
	// Username is the SFTP login name.
	Username string
	// Password is used for password authentication. It may be empty when using ClientPrivateKey.
	Password string

	// Timeout is the SSH handshake and connection timeout. The zero value means no timeout.
	Timeout time.Duration
	// MaxConnections is the maximum number of concurrent requests per file.
	// If it is zero or negative, NewClient defaults it to 1.
	MaxConnections int
	// PacketSize is the maximum SFTP packet size in bytes. The zero value uses the library default.
	PacketSize int

	// HostPublicKey configures an SSH public key to validate the remote server's host key.
	// If provided, this key will be merged into HostPublicKeys.
	// Deprecated: Use HostPublicKeys instead.
	HostPublicKey string

	// HostPublicKeys configures multiple SSH public keys to validate the remote server's host key.
	// Any key provided in HostPublicKey will be appended to this list.
	// Each element may be an authorized_keys line (the ssh-keyscan default) or a base64-encoded key.
	// If HostPublicKey and HostPublicKeys are both empty, host-key checking is disabled
	// (ssh.InsecureIgnoreHostKey) and a warning is logged.
	HostPublicKeys []string

	// ClientPrivateKey is the client private key for public-key authentication.
	// It may be a base64-encoded string or raw PEM.
	ClientPrivateKey string
	// ClientPrivateKeyPassword is the passphrase for an encrypted ClientPrivateKey.
	// It is a plain string, not base64 encoded.
	ClientPrivateKeyPassword string

	// SkipChmodAfterUpload, when true, does not chmod uploaded files to 0600.
	SkipChmodAfterUpload bool
	// SkipDirectoryCreation, when true, does not create missing parent directories on upload.
	SkipDirectoryCreation bool
	// SkipSyncAfterUpload, when true, does not fsync after writing an uploaded file.
	SkipSyncAfterUpload bool
}

// HostKeys returns the list of configured public keys to use for host key verification.
func (cfg ClientConfig) HostKeys() []string {
	if cfg.HostPublicKey != "" {
		cfg.HostPublicKeys = append(cfg.HostPublicKeys, cfg.HostPublicKey)
	}

	return dedupe(cfg.HostPublicKeys)
}

func dedupe[T comparable](vals []T) []T {
	seen := make(map[T]struct{})
	var out []T
	for i := range vals {
		if _, ok := seen[vals[i]]; ok {
			continue
		}
		seen[vals[i]] = struct{}{}
		out = append(out, vals[i])
	}
	return out
}
