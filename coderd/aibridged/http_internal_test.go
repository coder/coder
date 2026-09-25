package aibridged

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcerr"

	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
)

func TestAuthorizationErrorFromDRPC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code uint64
		kind intercept.AuthorizationErrorKind
	}{
		{name: "authentication", code: proto.AuthorizationErrorAuthentication, kind: intercept.AuthorizationErrorAuthentication},
		{name: "policy", code: proto.AuthorizationErrorPolicy, kind: intercept.AuthorizationErrorPolicy},
		{name: "evaluation", code: proto.AuthorizationErrorEvaluation, kind: intercept.AuthorizationErrorEvaluation},
		{name: "malformed", code: proto.AuthorizationErrorMalformed, kind: intercept.AuthorizationErrorMalformed},
		{name: "unknown", code: 9999, kind: intercept.AuthorizationErrorEvaluation},
		{name: "untyped", kind: intercept.AuthorizationErrorEvaluation},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cause := xerrors.New("test")
			err := authorizationErrorFromDRPC(drpcerr.WithCode(cause, tc.code))
			require.Equal(t, tc.kind, err.Kind)
			require.ErrorIs(t, err, cause)
		})
	}
}

func TestAuthorizationOutcomeForError(t *testing.T) {
	t.Parallel()
	require.Equal(t, metrics.AuthorizationOutcomeDenied, authorizationOutcomeForError(intercept.AuthorizationErrorAuthentication))
	require.Equal(t, metrics.AuthorizationOutcomeDenied, authorizationOutcomeForError(intercept.AuthorizationErrorPolicy))
	require.Equal(t, metrics.AuthorizationOutcomeError, authorizationOutcomeForError(intercept.AuthorizationErrorEvaluation))
	require.Equal(t, metrics.AuthorizationOutcomeError, authorizationOutcomeForError(intercept.AuthorizationErrorMalformed))
}
