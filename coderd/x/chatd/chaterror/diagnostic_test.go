package chaterror_test

import (
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := chaterror.FormatDiagnosticDetail(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}
