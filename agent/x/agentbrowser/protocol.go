package agentbrowser

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net/url"
	"strings"

	"golang.org/x/xerrors"
)

// The viewer stream is a byte stream, not a sequence of WebSocket
// messages, because coderd relays it with a plain byte copy. Every frame
// is [type u8][length u32 big-endian][payload].
const (
	frameTypeVideo byte = 1 // payload: [flags u8][encoded chunk]
	frameTypeJSON  byte = 2 // payload: JSON message

	videoFlagKey byte = 1

	frameHeaderSize = 5
	// maxClientFrame bounds frames read from viewers; they only send
	// small JSON control messages.
	maxClientFrame = 1 << 20
)

func encodeFrame(typ byte, payload []byte) []byte {
	buf := make([]byte, frameHeaderSize+len(payload))
	buf[0] = typ
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(payload))) //nolint:gosec // Payloads are far below 4 GiB.
	copy(buf[frameHeaderSize:], payload)
	return buf
}

func encodeJSONFrame(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		// All server messages are plain structs; this cannot fail.
		panic(err)
	}
	return encodeFrame(frameTypeJSON, data)
}

func readFrame(r io.Reader) (byte, []byte, error) {
	var hdr [frameHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:5])
	if n > maxClientFrame {
		return 0, nil, xerrors.Errorf("frame too large: %d bytes", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return hdr[0], payload, nil
}

// ServerConfig tells viewers how to configure their video decoder.
type ServerConfig struct {
	Type   string `json:"type"` // "config"
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// ServerState describes the content tab's navigation state.
type ServerState struct {
	Type         string `json:"type"` // "state"
	URL          string `json:"url"`
	Title        string `json:"title"`
	Loading      bool   `json:"loading"`
	CanGoBack    bool   `json:"can_go_back"`
	CanGoForward bool   `json:"can_go_forward"`
}

// ServerError reports a problem to viewers.
type ServerError struct {
	Type    string `json:"type"` // "error"
	Message string `json:"message"`
}

// ClientMessage is any message a viewer sends. Fields are used depending
// on Type.
type ClientMessage struct {
	Type string `json:"type"`

	// resize
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`

	// navigate
	URL string `json:"url,omitempty"`

	// mouse: Action is down, up, move or wheel.
	Action     string  `json:"action,omitempty"`
	X          float64 `json:"x,omitempty"`
	Y          float64 `json:"y,omitempty"`
	Button     string  `json:"button,omitempty"`
	Buttons    int64   `json:"buttons,omitempty"`
	ClickCount int64   `json:"click_count,omitempty"`
	DeltaX     float64 `json:"delta_x,omitempty"`
	DeltaY     float64 `json:"delta_y,omitempty"`

	// key: Action is down or up.
	Key     string `json:"key,omitempty"`
	Code    string `json:"code,omitempty"`
	KeyCode int64  `json:"key_code,omitempty"`

	// key, insert_text
	Text string `json:"text,omitempty"`

	// mouse, key: bit field alt=1, ctrl=2, meta=4, shift=8 (CDP).
	Modifiers int64 `json:"modifiers,omitempty"`
}

// normalizeURL accepts absolute http(s) URLs and bare host[:port][/path]
// input, which is treated as http. Anything else, such as search terms or
// other schemes, is rejected.
func normalizeURL(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" || strings.ContainsAny(s, " \t\r\n") {
		return "", xerrors.New("enter a URL")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", xerrors.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", xerrors.Errorf("unsupported URL scheme %q, use http or https", u.Scheme)
	}
	if u.Hostname() == "" {
		return "", xerrors.New("URL has no host")
	}
	return u.String(), nil
}
