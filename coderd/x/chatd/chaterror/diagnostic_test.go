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
			name: "DropsURLUserinfoQueryAndFragment",
			err: &url.Error{
				Op:  "Post",
				URL: "https://test-user:test-password@gateway.internal/v1/chat?test_token=test-value#fragment",
				Err: xerrors.New("unexpected EOF"),
			},
		},
		{
			name: "DropsWrappedURLError",
			err: xerrors.Errorf("stream failed: %w", &url.Error{
				Op:  "Get",
				URL: "https://test-key@gateway.internal/v1/chat?test_token=test-value",
				Err: xerrors.New("connection refused"),
			}),
		},
		{
			name: "DropsFlattenedURL",
			err:  xerrors.New(`Post "https://test-user:test-password@gateway.internal/v1/chat?test_token=test-value#fragment": unexpected EOF`),
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
