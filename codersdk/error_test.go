package codersdk_test

import (
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

func TestIsConnectionErr(t *testing.T) {
	t.Parallel()

	type tc = struct {
		name           string
		err            error
		expectedResult bool
	}

	cases := []tc{
		{
			// E.g. "no such host"
			name: "DNSError",
			err: &net.DNSError{
				Err:         "no such host",
				Name:        "foofoo",
				Server:      "1.1.1.1:53",
				IsTimeout:   false,
				IsTemporary: false,
				IsNotFound:  true,
			},
			expectedResult: true,
		},
		{
			// E.g. "connection refused"
			name: "OpErr",
			err: &net.OpError{
				Op:     "dial",
				Net:    "tcp",
				Source: nil,
				Addr:   nil,
				Err:    &os.SyscallError{},
			},
			expectedResult: true,
		},
		{
			name:           "OpaqueError",
			err:            xerrors.Errorf("I'm opaque!"),
			expectedResult: false,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, c.expectedResult, codersdk.IsConnectionErr(c.err))
		})
	}
}
