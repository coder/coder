package agentegress

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

const exitNodeRetryBackoff = 5 * time.Second

type exitNodeFailure struct {
	retryAt time.Time
}

type exitNodeSelector struct {
	mu       sync.Mutex
	logger   slog.Logger
	clock    quartz.Clock
	addrs    []netip.AddrPort
	current  int
	failures []exitNodeFailure
}

func newExitNodeSelector(logger slog.Logger, clock quartz.Clock, addrs []netip.AddrPort) *exitNodeSelector {
	if clock == nil {
		clock = quartz.NewReal()
	}
	return &exitNodeSelector{
		logger:   logger,
		clock:    clock,
		addrs:    append([]netip.AddrPort(nil), addrs...),
		failures: make([]exitNodeFailure, len(addrs)),
	}
}

func (s *exitNodeSelector) dial(ctx context.Context, dialer Dialer) (net.Conn, netip.AddrPort, error) {
	s.mu.Lock()
	now := s.clock.Now("exit_node_selector")
	current := s.current
	order := make([]int, 0, len(s.addrs))
	for i := range current {
		if !now.Before(s.failures[i].retryAt) {
			order = append(order, i)
			// Reserve this recovery probe so concurrent dials do not probe the
			// same preferred node before the backoff elapses again.
			s.failures[i].retryAt = now.Add(exitNodeRetryBackoff)
		}
	}
	if !now.Before(s.failures[current].retryAt) {
		order = append(order, current)
	}
	for i := current + 1; i < len(s.addrs); i++ {
		if !now.Before(s.failures[i].retryAt) {
			order = append(order, i)
		}
	}
	if len(order) == 0 {
		order = append(order, current)
	}
	s.mu.Unlock()

	var lastErr error
	for _, i := range order {
		conn, err := dialer.DialContextTCP(ctx, s.addrs[i])
		if err != nil {
			s.markFailure(s.addrs[i])
			lastErr = err
			continue
		}
		return conn, s.addrs[i], nil
	}
	return nil, netip.AddrPort{}, xerrors.Errorf("dial exit nodes: %w", lastErr)
}

func (s *exitNodeSelector) len() int {
	return len(s.addrs)
}

func (s *exitNodeSelector) markFailure(addr netip.AddrPort) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, candidate := range s.addrs {
		if candidate == addr {
			s.failures[i].retryAt = s.clock.Now("exit_node_failure").Add(exitNodeRetryBackoff)
			return
		}
	}
}

func (s *exitNodeSelector) markHealthy(ctx context.Context, addr netip.AddrPort) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, candidate := range s.addrs {
		if candidate != addr {
			continue
		}
		s.failures[i] = exitNodeFailure{}
		previous := s.current
		s.current = i
		if i != previous {
			s.logger.Info(ctx, "switched egress exit node", slog.F("from", s.addrs[previous]), slog.F("to", addr))
			return true
		}
		return false
	}
	return false
}
