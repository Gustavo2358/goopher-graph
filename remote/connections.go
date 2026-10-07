package remote

import (
	"net"
	"sync"
	"sync/atomic"
)

// Bound all connections, not just HTTP/2 streams within one connection.
// At saturation close newly accepted connections before TLS/HTTP2 allocation.
type boundedListener struct {
	net.Listener
	slots    chan struct{}
	active   *atomic.Int64
	rejected *atomic.Uint64
}

func (l *boundedListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.slots <- struct{}{}:
			l.active.Add(1)
			return &boundedConn{Conn: conn, release: func() { <-l.slots; l.active.Add(-1) }}, nil
		default:
			l.rejected.Add(1)
			_ = conn.Close()
		}
	}
}

type boundedConn struct {
	net.Conn
	once    sync.Once
	release func()
	err     error
}

func (c *boundedConn) Close() error {
	c.once.Do(func() { c.err = c.Conn.Close(); c.release() })
	return c.err
}
