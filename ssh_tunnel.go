package sshtunnel

import (
	"errors"
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

func (t *SSHTunnel) Listen() (net.Listener, error) {
	return net.Listen("tcp", t.Local.String())
}

func (t *SSHTunnel) Start() error {
	listener, err := t.Listen()
	if err != nil {
		t.logf("listen error: %s", err)
		return err
	}
	defer listener.Close()

	return t.Serve(listener)
}

func (tunnel *SSHTunnel) Serve(listener net.Listener) error {

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
		tunnel.logf("listening for new connections on %s:%d...", tunnel.Local.Host, tunnel.Local.Port)

		select {
		case <-tunnel.close:
			tunnel.logf("close signal received, closing...")
			tunnel.isOpen = false
		case conn := <-c:
			tunnel.Conns = append(tunnel.Conns, conn)
			tunnel.logf("accepted connection from %s", conn.RemoteAddr().String())
			go tunnel.forward(conn)
		}
	}
	var total int
	total = len(tunnel.Conns)
	for i, conn := range tunnel.Conns {
		tunnel.logf("closing the netConn (%d of %d)", i+1, total)
		err := conn.Close()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				// no need to report on closed connections
				continue
			}
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

				if err := localConn.Close(); err != nil {
					tunnel.logf("failed to close local connection: %v", err)
					return
				}

				tunnel.logf("dial failed, closing local connection: %v", err)
				return
			}
			tunnel.logf("server dial error: %v: attempt %d/%d", err, tunnel.MaxConnectionAttempts-attemptsLeft, tunnel.MaxConnectionAttempts)
		} else {
			break
		}
	}

	tunnel.logf("connected to %s (1 of 2)\n", tunnel.Server.String())
	tunnel.SvrConns = append(tunnel.SvrConns, serverConn)

	remoteConn, err := serverConn.Dial("tcp", tunnel.Remote.String())
	if err != nil {
		tunnel.logf("remote dial error: %s", err)

		if err := serverConn.Close(); err != nil {
			tunnel.logf("failed to close server connection: %v", err)
		}
		if err := localConn.Close(); err != nil {
			tunnel.logf("failed to close local connection: %v", err)
		}
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
}

func (tunnel *SSHTunnel) Close() {
	tunnel.close <- struct{}{}
}

// NewSSHTunnel creates a new single-use tunnel. Supplying "0" for localport will use a random port.
func NewSSHTunnel(tunnel string, auth ssh.AuthMethod, destination string, localport string) (*SSHTunnel, error) {

	localEndpoint, err := NewEndpoint("localhost:" + localport)
	if err != nil {
		return nil, err
	}

	server, err := NewEndpoint(tunnel)
	if err != nil {
		return nil, err
	}
	if server.Port == 0 {
		server.Port = 22
	}

	remoteEndpoint, err := NewEndpoint(destination)
	if err != nil {
		return nil, err
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
		Remote: remoteEndpoint,
		close:  make(chan interface{}),
	}

	return sshTunnel, nil
}
