package go_sftp

import "time"

type ClientConfig struct {
	Hostname string
	Username string
	Password string

	Timeout        time.Duration
	MaxConnections int
	PacketSize     int

	// HostPublicKey configures an SSH public key to validate the remote server's host key.
	// If provided, this key will be merged into HostPublicKeys.
	// Deprecated: Use HostPublicKeys instead.
	HostPublicKey string

	// HostPublicKeys configures multiple SSH public keys to validate the remote server's host key.
	// Any key provided in HostPublicKey will be appended to this list.
	HostPublicKeys []string

	// ClientPrivateKey must be a base64 encoded string
	ClientPrivateKey         string
	ClientPrivateKeyPassword string // not base64 encoded

	SkipChmodAfterUpload  bool
	SkipDirectoryCreation bool
	SkipSyncAfterUpload   bool
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
