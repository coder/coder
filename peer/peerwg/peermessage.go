package peerwg

import (
	"bytes"
	"strconv"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"inet.af/netaddr"
	"tailscale.com/types/key"
)

const handshakeSeparator byte = '\n'

// Handshake is a message received from a wireguard peer, indicating
// it would like to connect.
type Handshake struct {
	// Recipient is the uuid of the agent that the message was intended for.
	Recipient uuid.UUID `json:"recipient"`
	// DiscoPublicKey is the disco public key of the peer.
	DiscoPublicKey key.DiscoPublic `json:"disco"`
	// NodePublicKey is the public key of the peer.
	NodePublicKey key.NodePublic `json:"public"`
	// IPv6 is the IPv6 address of the peer.
	IPv6 netaddr.IP `json:"ipv6"`
}

// HandshakeRecipientHint parses the first part of a serialized
// Handshake to quickly determine if the message is meant for the
// provided recipient.
func HandshakeRecipientHint(agentID []byte, msg []byte) (bool, error) {
	idx := bytes.Index(msg, []byte{handshakeSeparator})
	if idx == -1 {
		return false, xerrors.Errorf("invalid peer message, no separator")
	}

	return bytes.Equal(agentID, msg[:idx]), nil
}

func (h *Handshake) UnmarshalText(text []byte) error {
	sp := bytes.Split(text, []byte{handshakeSeparator})
	if len(sp) != 4 {
		return xerrors.Errorf("expected 4 parts, got %d", len(sp))
	}

	err := h.Recipient.UnmarshalText(sp[0])
	if err != nil {
		return xerrors.Errorf("parse recipient: %w", err)
	}

	err = h.DiscoPublicKey.UnmarshalText(sp[1])
	if err != nil {
		return xerrors.Errorf("parse disco: %w", err)
	}

	err = h.NodePublicKey.UnmarshalText(sp[2])
	if err != nil {
		return xerrors.Errorf("parse public: %w", err)
	}

	h.IPv6, err = netaddr.ParseIP(string(sp[3]))
	if err != nil {
		return xerrors.Errorf("parse ipv6: %w", err)
	}

	return nil
}

func (h Handshake) MarshalText() ([]byte, error) {
	const expectedLen = 223
	var buf bytes.Buffer
	buf.Grow(expectedLen)

	recp, _ := h.Recipient.MarshalText()
	_, _ = buf.Write(recp)
	_ = buf.WriteByte(handshakeSeparator)

	disco, _ := h.DiscoPublicKey.MarshalText()
	_, _ = buf.Write(disco)
	_ = buf.WriteByte(handshakeSeparator)

	pub, _ := h.NodePublicKey.MarshalText()
	_, _ = buf.Write(pub)
	_ = buf.WriteByte(handshakeSeparator)

	ipv6 := h.IPv6.StringExpanded()
	_, _ = buf.WriteString(ipv6)

	// Ensure we're always allocating exactly enough.
	if buf.Len() != expectedLen {
		panic("buffer length mismatch: want 221, got " + strconv.Itoa(buf.Len()))
	}
	return buf.Bytes(), nil
}
