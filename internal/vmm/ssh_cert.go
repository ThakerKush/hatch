package vmm

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

// LoadCA reads an OpenSSH private key file and returns an ssh.Signer
// suitable for signing SSH certificates.
func LoadCA(privateKeyPath string) (ssh.Signer, error) {
	data, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read CA private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("parse CA private key: %w", err)
	}
	return signer, nil
}

// SignCert creates a short-lived SSH user certificate for the given public
// key. The certificate is scoped to the specified VM via ValidPrincipals,
// so it can only authenticate on VMs whose auth_principals file contains
// the same principal value.
func SignCert(ca ssh.Signer, pubKey ssh.PublicKey, vmID string, ttl time.Duration) (*ssh.Certificate, error) {
	serial, err := randomSerial()
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	cert := &ssh.Certificate{
		CertType:        ssh.UserCert,
		Key:             pubKey,
		KeyId:           fmt.Sprintf("hatch:%s", vmID),
		ValidPrincipals: []string{vmID},
		ValidAfter:      uint64(now.Add(-30 * time.Second).Unix()),
		ValidBefore:     uint64(now.Add(ttl).Unix()),
		Serial:          serial,
		Permissions: ssh.Permissions{
			Extensions: map[string]string{
				"permit-pty": "",
			},
		},
	}

	if err := cert.SignCert(rand.Reader, ca); err != nil {
		return nil, fmt.Errorf("sign certificate: %w", err)
	}
	return cert, nil
}

func randomSerial() (uint64, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf[:]), nil
}
