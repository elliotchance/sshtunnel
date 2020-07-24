package sshtunnel

import (
	"io"
	"log"
	"net"

	"golang.org/x/crypto/ssh"
)

type SSHTunnel struct {
	Local    *Endpoint
	Server   *Endpoint
	Remote   *Endpoint
	Config   *ssh.ClientConfig
	Log      *log.Logger
	Conns    []net.Conn
	SvrConns []*ssh.Client
	isOpen   bool
	close    chan interface{}
}

func (tunnel *SSHTunnel) logf(fmt string, args ...interface{}) {
	if tunnel.Log != nil {
		tunnel.Log.Printf(fmt, args...)
	}
}

func newConnectionWaiter(listener net.Listener, c chan net.Conn) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	c <- conn
}

func (tunnel *SSHTunnel) Start() error {
	listener, err := net.Listen("tcp", tunnel.Local.String())
	if err != nil {
		return err
	}
	tunnel.isOpen = true
	tunnel.Local.Port = listener.Addr().(*net.TCPAddr).Port

	for {
		if !tunnel.isOpen {
			break
		}

		c := make(chan net.Conn)
		go newConnectionWaiter(listener, c)
		tunnel.logf("listening for new connections...")

		select {
		case <-tunnel.close:
			tunnel.logf("close signal received, closing...")
			tunnel.isOpen = false
		case conn := <-c:
			tunnel.Conns = append(tunnel.Conns, conn)
			tunnel.logf("accepted connection")
			go tunnel.forward(conn)
		}
	}
	var total int
	total = len(tunnel.Conns)
	for i, conn := range tunnel.Conns {
		tunnel.logf("closing the netConn (%d of %d)", i+1, total)
		err := conn.Close()
		if err != nil {
			tunnel.logf(err.Error())
		}
	}
	total = len(tunnel.SvrConns)
	for i, conn := range tunnel.SvrConns {
		tunnel.logf("closing the serverConn (%d of %d)", i+1, total)
		err := conn.Close()
		if err != nil {
			tunnel.logf(err.Error())
		}
	}
	err = listener.Close()
	if err != nil {
		return err
	}
	tunnel.logf("tunnel closed")
	return nil
}

func (tunnel *SSHTunnel) forward(localConn net.Conn) {
	serverConn, err := ssh.Dial("tcp", tunnel.Server.String(), tunnel.Config)
	if err != nil {
		tunnel.logf("server dial error: %s", err)
		return
	}
	tunnel.logf("connected to %s (1 of 2)\n", tunnel.Server.String())
	remoteConn, err := serverConn.Dial("tcp", tunnel.Remote.String())
	if err != nil {
		tunnel.logf("remote dial error: %s", err)
		return
	}
	tunnel.Conns = append(tunnel.Conns, remoteConn)
	tunnel.SvrConns = append(tunnel.SvrConns, serverConn)
	tunnel.logf("connected to %s (2 of 2)\n", tunnel.Remote.String())
	copyConn := func(writer, reader net.Conn) {
		_, err := io.Copy(writer, reader)
		if err != nil {
			tunnel.logf("io.Copy error: %s", err)
		}
	}
	go copyConn(localConn, remoteConn)
	go copyConn(remoteConn, localConn)

	return
}

func (tunnel *SSHTunnel) Close() {
	tunnel.close <- struct{}{}
	return
}

// NewSSHTunnel creates a new single-use tunnel. Supplying "0" for localport will use a random port.
func NewSSHTunnel(tunnel string, auth ssh.AuthMethod, destination string, localport string) *SSHTunnel {

	localEndpoint := NewEndpoint("localhost:" + localport)

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
		close:  make(chan interface{}),
	}

	return sshTunnel
}
