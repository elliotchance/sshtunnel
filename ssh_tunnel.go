package sshtunnel

import (
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"net"
	"strconv"
)

type SSHTunnel struct {
	Local  *Endpoint
	Server *Endpoint
	Remote *Endpoint
	Config *ssh.ClientConfig
	Log    *log.Logger
}

func (tunnel *SSHTunnel) logf(fmt string, args ...interface{}) {
	if tunnel.Log != nil {
		tunnel.Log.Printf(fmt, args...)
	}
}

func (tunnel *SSHTunnel) Start() error {
	listener, err := net.Listen("tcp", tunnel.Local.String())
	if err != nil {
		return err
	}
	defer listener.Close()

	tunnel.Local.Port = listener.Addr().(*net.TCPAddr).Port

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}

		tunnel.logf("accepted connection")
		go tunnel.forward(conn)
	}
}

func (tunnel *SSHTunnel) forward(localConn net.Conn) {
	var serverConn *ssh.Client
	var serverErr interface{}

	var retry int
	for {

		retry ++
		serverConn, serverErr = ssh.Dial("tcp", tunnel.Server.String(), tunnel.Config)
		if serverErr != nil {
			tunnel.logf("server dial error: %s", serverErr)
		} else {
			break
		}
	}

	if retry > 1 {
		tunnel.logf("Retry server: %s", strconv.Itoa(retry))
	}

	tunnel.logf("connected to %s (1 of 2)\n", tunnel.Server.String())

	var remoteConn net.Conn
	var remoteError interface{}
	retry = 0

	for {
		retry ++
		remoteConn, remoteError = serverConn.Dial("tcp", tunnel.Remote.String())
		if remoteError != nil {
			tunnel.logf("remote dial error: %s", remoteError)
		}else{

			break
		}
	}
			
	if retry > 1 {
		tunnel.logf("Retry remote: %s", strconv.Itoa(retry))
	}

	tunnel.logf("connected to %s (2 of 2)\n", tunnel.Remote.String())

	go func(writer, reader net.Conn) {
		defer writer.Close()
		defer reader.Close()
		_, err := io.Copy(writer, reader)
		if err != nil {
			tunnel.logf("io.Copy local to remote warm: %s", err)
		}
	}(localConn, remoteConn)

	go func(writer, reader net.Conn) {
		defer writer.Close()
		defer reader.Close()
		_, err := io.Copy(writer, reader)
		if err != nil {
			tunnel.logf("io.Copy remote to local warm: %s", err)
		}
	}(remoteConn, localConn)

}

func NewSSHTunnel(tunnel string, auth ssh.AuthMethod, destination string, localPort string) *SSHTunnel {
	// A random port will be chosen for us.
	localEndpoint := NewEndpoint("localhost:" + localPort)

	server := NewEndpoint(tunnel)
	if server.Port == 0 {
		server.Port = 22
	}

	sshTunnel := &SSHTunnel{
		Config: &ssh.ClientConfig{
			User: server.User,
			Auth: []ssh.AuthMethod{auth},
			HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				// Always accept key.
				return nil
			},
		},
		Local:  localEndpoint,
		Server: server,
		Remote: NewEndpoint(destination),
	}

	return sshTunnel
}

