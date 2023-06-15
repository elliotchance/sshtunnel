package sshtunnel

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type Endpoint struct {
	Host string
	Port int
	User string
}

func NewEndpoint(s string) *Endpoint {
	endpoint := &Endpoint{
		Host: s,
	}

	if parts := strings.Split(endpoint.Host, "@"); len(parts) > 1 {
		endpoint.User = parts[0]
		endpoint.Host = parts[1]
	}

	host, port, err := net.SplitHostPort(endpoint.Host)
	if err == nil {
		endpoint.Host = host
		endpoint.Port, _ = strconv.Atoi(port)
	}

	return endpoint
}

func (endpoint *Endpoint) String() string {
	return fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)
}
