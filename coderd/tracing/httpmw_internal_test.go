package tracing

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidSessionID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		id    string
		valid bool
	}{
		{"LowerHex", "0123456789abcdef0123456789abcdef", true},
		{"UpperHex", "0123456789ABCDEF0123456789ABCDEF", true},
		{"Empty", "", false},
		{"TooShort", "0123456789abcdef0123456789abcde", false},
		{"TooLong", "0123456789abcdef0123456789abcdef0", false},
		{"NonHex", "0123456789abcdef0123456789abcdeg", false},
		{"Uuid", "0123456789ab-cdef-0123456789abcde", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, c.valid, validSessionID(c.id))
		})
	}
}

func TestSessionIDFromHeaders(t *testing.T) {
	t.Parallel()

	const validID = "0123456789abcdef0123456789abcdef"

	cases := []struct {
		name    string
		baggage string
		want    string
	}{
		{"Valid", "session_id=" + validID, validID},
		{"WithOtherMembers", "foo=bar,session_id=" + validID + ",baz=qux", validID},
		{"Missing", "foo=bar", ""},
		{"NoHeader", "", ""},
		{"Malformed", "session_id=not-a-hex-value", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			h := http.Header{}
			if c.baggage != "" {
				h.Set("baggage", c.baggage)
			}
			require.Equal(t, c.want, sessionIDFromHeaders(h))
		})
	}
}
