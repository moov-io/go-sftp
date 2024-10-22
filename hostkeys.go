package go_sftp

import (
	"bytes"
	"fmt"
	"net"

	"github.com/moov-io/go-sftp/pkg/sshx"
	"golang.org/x/crypto/ssh"
)

type MultiKeyCallback struct {
	hostKeys []ssh.PublicKey
}

func NewMultiKeyCallback(keys []string) (ssh.HostKeyCallback, error) {
	m := &MultiKeyCallback{}
	for i := range keys {
		pubKey, err := sshx.ReadPubKey([]byte(keys[i]))
		if err != nil {
			return nil, fmt.Errorf("sftp: reading host key at index %d: %w", i, err)
		}
		m.hostKeys = append(m.hostKeys, pubKey)
	}
	return m.check, nil
}

// check is an ssh.HostKeyCallback based on ssh.FixedHostKey, running the equality check against each configured key.
func (m *MultiKeyCallback) check(_ string, _ net.Addr, key ssh.PublicKey) error {
	for _, mKey := range m.hostKeys {
		if bytes.Equal(key.Marshal(), mKey.Marshal()) {
			return nil
		}
	}
	return fmt.Errorf("sftp: no matching host keys")
}
