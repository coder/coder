package vncproxy

import "sync"

// rfbVersion is the negotiated RFB protocol version. The handshake
// observes the strings the two sides exchange and uses the lower of
// the two as required by RFC 6143.
type rfbVersion int

const (
	// rfbVersionUnknown means ProtocolVersion has not been observed
	// yet. Parsers must never branch on this value at runtime; it is
	// only the zero value before the handshake begins.
	rfbVersionUnknown rfbVersion = iota
	rfbVersion33
	rfbVersion37
	rfbVersion38
)

// session holds connection-scoped state derived during the handshake.
// After handshake completion only bpp is read concurrently from both
// the C2S and S2C goroutines, guarded by a mutex. All other fields are
// written exactly once during the handshake and then read-only
// thereafter, so post-handshake reads do not need synchronization.
type session struct {
	// version is the negotiated RFB protocol version, set during the
	// handshake.
	version rfbVersion
	// bpp is the current pixel depth in bits. Initialized from
	// ServerInit and updated on each C2S SetPixelFormat.
	bpp   int
	bppMu sync.Mutex
}

func newSession() *session {
	return &session{bpp: 32}
}

func (s *session) getBPP() int {
	s.bppMu.Lock()
	defer s.bppMu.Unlock()
	return s.bpp
}

func (s *session) setBPP(bpp int) {
	s.bppMu.Lock()
	defer s.bppMu.Unlock()
	s.bpp = bpp
}
