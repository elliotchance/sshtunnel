package sshtunnel

import (
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func SSHAgent() ssh.AuthMethod {
	sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		panic("connecting to SSH agent: " + err.Error())
	}
	return ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers)
}
