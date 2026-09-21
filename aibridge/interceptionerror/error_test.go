package interceptionerror_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/interceptionerror"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
)

type categorizer struct {
	result *recorder.ErrorType
}

func (c categorizer) CategorizeError(error) *recorder.ErrorType { return c.result }

func TestCategorize(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		cat      interceptionerror.Categorizer
		err      error
		status   int
		wantType recorder.ErrorType
		wantMsg  string
	}{
		{name: "Success", wantType: "", wantMsg: ""},
		{name: "Timeout", err: context.DeadlineExceeded, wantType: recorder.ErrorTypeTimeout, wantMsg: context.DeadlineExceeded.Error()},
		{name: "RateLimited", err: &keypool.Error{Kind: keypool.ErrorKindRateLimited}, wantType: recorder.ErrorTypeRateLimited, wantMsg: (&keypool.Error{Kind: keypool.ErrorKindRateLimited}).Error()},
		{name: "HTTPStatusFallback", status: http.StatusForbidden, wantType: recorder.ErrorTypeUnauthorized, wantMsg: http.StatusText(http.StatusForbidden)},
		{name: "Provider", cat: categorizer{result: new(recorder.ErrorTypeOverloaded)}, err: xerrors.New("provider error"), wantType: recorder.ErrorTypeOverloaded, wantMsg: "provider error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotType, gotMsg := interceptionerror.Categorize(tc.cat, tc.err, tc.status)
			require.Equal(t, tc.wantType, gotType)
			require.Equal(t, tc.wantMsg, gotMsg)
		})
	}
}

func TestCategorizeTruncatesMessage(t *testing.T) {
	t.Parallel()

	message := strings.Repeat("a", 2048)
	_, got := interceptionerror.Categorize(nil, xerrors.New(message), 0)
	require.Len(t, got, 1024)

	multibyte := strings.Repeat("€", 1024)
	_, got = interceptionerror.Categorize(nil, xerrors.New(multibyte), 0)
	require.True(t, utf8.ValidString(got))
	require.Less(t, len(got), 1024)
}
