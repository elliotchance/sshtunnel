package sshtunnel

import "golang.org/x/crypto/ssh"

type endpointProvider func() *Endpoint
type sshConfigProvider func() *ssh.ClientConfig

type sshClientManager interface {
	ensureConnection() (*ssh.Client, error)
	cleanup() error
}

type multipleSSHClientsManager struct {
	endpointProvider  endpointProvider
	sshConfigProvider sshConfigProvider

	log logger

	maxConnectionAttempts int
	svrConns              []*ssh.Client
}

func (m *multipleSSHClientsManager) ensureConnection() (*ssh.Client, error) {
	var (
		serverConn *ssh.Client
		err        error
	)
	attemptsLeft := m.maxConnectionAttempts
	for {
		serverConn, err = ssh.Dial("tcp", m.endpointProvider().String(), m.sshConfigProvider())
		if err != nil {
			attemptsLeft--

			if attemptsLeft <= 0 {
				m.log.Printf("server dial error: %v: exceeded %d attempts", err, m.maxConnectionAttempts)
				return nil, err
			}
			m.log.Printf("server dial error: %v: attempt %d/%d", err, m.maxConnectionAttempts-attemptsLeft, m.maxConnectionAttempts)
		} else {
			break
		}
	}
	m.svrConns = append(m.svrConns, serverConn)
	return serverConn, nil
}

func (m *multipleSSHClientsManager) cleanup() error {
	total := len(m.svrConns)
	for i, conn := range m.svrConns {
		m.log.Printf("closing the serverConn (%d of %d)", i+1, total)
		err := conn.Close()
		if err != nil {
			m.log.Printf(err.Error())
		}
	}
	return nil
}

func newMultipleSSHClientsManager(endpointProvider endpointProvider, sshConfigProvider sshConfigProvider, log logger) sshClientManager {
	return &multipleSSHClientsManager{
		endpointProvider:  endpointProvider,
		sshConfigProvider: sshConfigProvider,
		log:               log,
	}
}
