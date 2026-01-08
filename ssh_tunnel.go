package sshtunnel

import (
	"errors"
	"io"
	"net"

	"golang.org/x/crypto/ssh"
)

type SSHTunnel struct {
	Local                 *Endpoint
	Server                *Endpoint
	Remote                *Endpoint
	Config                *ssh.ClientConfig
	Log                   logger
	log                   logger
	Conns                 []net.Conn
	sshClientManager      sshClientManager
	MaxConnectionAttempts int
	isOpen                bool
	close                 chan interface{}
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
		t.log.Printf("listen error: %s", err)
		return err
	}
	defer listener.Close()

	return t.Serve(listener)
}

func (t *SSHTunnel) Serve(listener net.Listener) error {

	t.isOpen = true
	t.Local.Port = listener.Addr().(*net.TCPAddr).Port

	// Ensure that MaxConnectionAttempts is at least 1. This check is done here
	// since the library user can set the value at any point before Start() is called,
	// and this check protects against the case where the programmer set MaxConnectionAttempts
	// to 0 for some reason.
	if t.MaxConnectionAttempts <= 0 {
		t.MaxConnectionAttempts = 1
	}

	for {
		if !t.isOpen {
			break
		}

		c := make(chan net.Conn)
		go newConnectionWaiter(listener, c)
		t.log.Printf("listening for new connections on %s:%d...", t.Local.Host, t.Local.Port)

		select {
		case <-t.close:
			t.log.Printf("close signal received, closing...")
			t.isOpen = false
		case conn := <-c:
			t.Conns = append(t.Conns, conn)
			t.log.Printf("accepted connection from %s", conn.RemoteAddr().String())
			go t.forward(conn)
		}
	}
	var total int
	total = len(t.Conns)
	for i, conn := range t.Conns {
		t.log.Printf("closing the netConn (%d of %d)", i+1, total)
		err := conn.Close()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				// no need to report on closed connections
				continue
			}
			t.log.Printf(err.Error())
		}
	}
	if err := t.sshClientManager.cleanup(); err != nil {
		t.log.Printf(err.Error())
	}

	t.log.Printf("tunnel closed")
	return nil
}

func (t *SSHTunnel) forward(localConn net.Conn) {
	serverConn, err := t.sshClientManager.ensureConnection()
	if err != nil {
		t.log.Printf("dial failed, closing local connection: %v", err)
		if err := localConn.Close(); err != nil {
			t.log.Printf("failed to close local connection: %v", err)
			return
		}
		return
	}

	t.log.Printf("connected to %s (1 of 2)\n", t.Server.String())

	remoteConn, err := serverConn.Dial("tcp", t.Remote.String())
	if err != nil {
		t.log.Printf("remote dial error: %s", err)

		if err := serverConn.Close(); err != nil {
			t.log.Printf("failed to close server connection: %v", err)
		}
		if err := localConn.Close(); err != nil {
			t.log.Printf("failed to close local connection: %v", err)
		}
		return
	}
	t.Conns = append(t.Conns, remoteConn)
	t.log.Printf("connected to %s (2 of 2)\n", t.Remote.String())
	copyConn := func(writer, reader net.Conn) {
		_, err := io.Copy(writer, reader)
		if err != nil {
			t.log.Printf("io.Copy error: %s", err)
		}
	}
	go copyConn(localConn, remoteConn)
	go copyConn(remoteConn, localConn)
}

func (t *SSHTunnel) Close() {
	t.close <- struct{}{}
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

	lw := &logWrapper{
		logProvider: func() logger { return sshTunnel.Log },
	}
	sshTunnel.log = lw

	sshTunnel.sshClientManager = newMultipleSSHClientsManager(
		func() *Endpoint { return sshTunnel.Server },
		func() *ssh.ClientConfig { return sshTunnel.Config },
		lw,
	)

	return sshTunnel, nil
}
