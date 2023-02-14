package sshtunnel

import (
	"io"
	"net"

	"golang.org/x/crypto/ssh"
)

type logger interface {
	Printf(string, ...interface{})
}

type SSHTunnel struct {
	Local                 *Endpoint
	Server                *Endpoint
	Remote                *Endpoint
	Config                *ssh.ClientConfig
	Log                   logger
	Conns                 []net.Conn
	SvrConns              []*ssh.Client
	MaxConnectionAttempts int
	isOpen                bool
	close                 chan interface{}
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

	// Ensure that MaxConnectionAttempts is at least 1. This check is done here
	// since the library user can set the value at any point before Start() is called,
	// and this check protects against the case where the programmer set MaxConnectionAttempts
	// to 0 for some reason.
	if tunnel.MaxConnectionAttempts <= 0 {
		tunnel.MaxConnectionAttempts = 1
	}

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
	var (
		serverConn   *ssh.Client
		err          error
		attemptsLeft int = tunnel.MaxConnectionAttempts
	)

	for {
		serverConn, err = ssh.Dial("tcp", tunnel.Server.String(), tunnel.Config)
		if err != nil {
			attemptsLeft--

			if attemptsLeft <= 0 {
				tunnel.logf("server dial error: %v: exceeded %d attempts", err, tunnel.MaxConnectionAttempts)
				return
			}
		} else {
			break
		}
	}

	tunnel.logf("connected to %s (1 of 2)\n", tunnel.Server.String())
	tunnel.SvrConns = append(tunnel.SvrConns, serverConn)

	remoteConn, err := serverConn.Dial("tcp", tunnel.Remote.String())
	if err != nil {
		tunnel.logf("remote dial error: %s", err)
		return
	}
	tunnel.Conns = append(tunnel.Conns, remoteConn)
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
