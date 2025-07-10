package backend

import (
	"fmt"
	"net"
)

// oneConnListener is a net.Listener that returns only one connection
type oneConnListener struct {
	conn     net.Conn
	done     chan struct{}
	returned bool
}

func newOneConnListener(conn net.Conn) net.Listener {
	return &oneConnListener{
		conn: conn,
		done: make(chan struct{}),
	}
}

func (l *oneConnListener) Accept() (net.Conn, error) {
	if l.returned {
		<-l.done // Wait forever
		return nil, fmt.Errorf("connection already accepted")
	}
	l.returned = true
	return l.conn, nil
}

func (l *oneConnListener) Close() error {
	close(l.done)
	return nil
}

func (l *oneConnListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}
