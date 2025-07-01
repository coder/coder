package testutil

import (
	"context"
	"net"
	"sync"

	"golang.org/x/xerrors"
)

type Addr struct {
	network string
	addr    string
}

func NewAddr(network, addr string) Addr {
	return Addr{network, addr}
}

func (a Addr) Network() string {
	return a.network
}

func (a Addr) Address() string {
	return a.addr
}

func (a Addr) String() string {
	return a.network + "|" + a.addr
}

type InProcNet struct {
	sync.Mutex

	listeners map[Addr]*inProcListener
}

type inProcListener struct {
	c chan net.Conn
	n *InProcNet
	a Addr
	o sync.Once
}

func NewInProcNet() *InProcNet {
	return &InProcNet{listeners: make(map[Addr]*inProcListener)}
}

func (n *InProcNet) Listen(network, address string) (net.Listener, error) {
	a := Addr{network, address}
	n.Lock()
	defer n.Unlock()
	if _, ok := n.listeners[a]; ok {
		return nil, xerrors.New("busy")
	}
	l := newInProcListener(n, a)
	n.listeners[a] = l
	return l, nil
}

func (n *InProcNet) Dial(ctx context.Context, a Addr) (net.Conn, error) {
	n.Lock()
	defer n.Unlock()
	l, ok := n.listeners[a]
	if !ok {
		return nil, xerrors.Errorf("nothing listening on %s", a)
	}
	x, y := net.Pipe()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case l.c <- x:
		return y, nil
	}
}

func newInProcListener(n *InProcNet, a Addr) *inProcListener {
	return &inProcListener{
		c: make(chan net.Conn),
		n: n,
		a: a,
	}
}

func (l *inProcListener) Accept() (net.Conn, error) {
	c, ok := <-l.c
	if !ok {
		return nil, net.ErrClosed
	}
	return c, nil
}

func (l *inProcListener) Close() error {
	l.o.Do(func() {
		l.n.Lock()
		defer l.n.Unlock()
		delete(l.n.listeners, l.a)
		close(l.c)
	})
	return nil
}

func (l *inProcListener) Addr() net.Addr {
	return l.a
}
