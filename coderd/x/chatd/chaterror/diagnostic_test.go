package chaterror_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
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
			err:  errors.New("stream response:\n\tconnection reset by peer"),
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

func TestFormatDiagnosticDetail_Truncates(t *testing.T) {
	t.Parallel()

	got := chaterror.FormatDiagnosticDetail(errors.New(strings.Repeat("x", 510)))
	require.Len(t, []rune(got), 500)
	require.True(t, strings.HasSuffix(got, "…"))
}

func TestClassify_GenericFallbackIncludesDiagnosticDetail(t *testing.T) {
	t.Parallel()

	classified := chaterror.Classify(xerrors.New(
		`stream response: Post "https://llm.example.com/v1/chat": decoder failed`,
	))

	require.Equal(t, codersdk.ChatErrorKindGeneric, classified.Kind)
	require.Equal(t, "The chat request failed unexpectedly.", classified.Message)
	require.Contains(t, classified.Detail, "decoder failed")
}
