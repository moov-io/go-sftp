package go_sftp

import "time"

type ClientConfig struct {
	Hostname string // remote address, including port (for example "sftp.example.com:22")
	Username string // SFTP login name
	Password string // used for password authentication; may be empty when using ClientPrivateKey

	Timeout        time.Duration // TCP connect timeout (ssh.ClientConfig.Timeout); zero means no timeout
	MaxConnections int           // must be >= 1; 0/negative errors from pkg/sftp after SSH connects
	PacketSize     int           // maximum SFTP packet size in bytes; zero uses the library default

	// HostPublicKey configures an SSH public key to validate the remote server's host key.
	// If provided, this key will be merged into HostPublicKeys.
	// Deprecated: Use HostPublicKeys instead.
	HostPublicKey string

	// HostPublicKeys configures multiple SSH public keys to validate the remote server's host key.
	// Any key provided in HostPublicKey will be appended to this list.
	// Each element may be an authorized_keys line (the ssh-keyscan default) or a base64-encoded key.
	// If HostPublicKey and HostPublicKeys are both empty, host-key checking is disabled
	// (ssh.InsecureIgnoreHostKey) and a warning is logged when the logger is non-nil.
	HostPublicKeys []string

	// ClientPrivateKey is the client private key for public-key authentication.
	// It may be a base64-encoded string or raw PEM.
	ClientPrivateKey         string
	ClientPrivateKeyPassword string // passphrase for an encrypted ClientPrivateKey; not base64 encoded

	SkipChmodAfterUpload  bool // when true, does not chmod uploaded files to 0600
	SkipDirectoryCreation bool // when true, does not create missing parent directories on upload
	SkipSyncAfterUpload   bool // when true, does not fsync after writing an uploaded file
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
