package sshtunnel

import (
	"os"

	"golang.org/x/crypto/ssh"
)

func PrivateKeyFile(file string, pass ...string) ssh.AuthMethod {
	buffer, err := os.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}

	return ssh.PublicKeys(key)
}

// All credits go to the author fagnercarvalho
func PrivateKeyFileWithPassphrase(file string, passphrase string) ssh.AuthMethod {
	buffer, err := os.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKeyWithPassphrase(buffer, []byte(passphrase))
	if err != nil {
		return nil
	}

	return ssh.PublicKeys(key)
}
