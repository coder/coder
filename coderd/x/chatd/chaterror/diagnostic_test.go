package chaterror_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
)

func TestFormatDiagnosticDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "Nil",
			err:  nil,
		},
		{
			name: "CollapsesWhitespace",
			err:  xerrors.New("stream response:\n\tconnection reset by peer"),
			want: "stream response: connection reset by peer",
		},
		{
			name: "RedactsURLUserinfoQueryAndFragment",
			err: &url.Error{
				Op:  "Post",
				URL: "https://user:password@gateway.internal/v1/chat?api_key=secret&x=ok#fragment",
				Err: xerrors.New("unexpected EOF"),
			},
			want: `Post "https://gateway.internal/v1/chat": unexpected EOF`,
		},
		{
			name: "RedactsWrappedURLError",
			err: xerrors.Errorf("stream failed: %w", &url.Error{
				Op:  "Get",
				URL: "https://secret-key@gateway.internal/v1/chat?token=secret",
				Err: xerrors.New("connection refused"),
			}),
			want: `stream failed: Get "https://gateway.internal/v1/chat": connection refused`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chaterror.FormatDiagnosticDetail(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}
