package aibridged

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcerr"

	"github.com/coder/coder/v2/aibridge/intercept"
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := authorizationErrorFromDRPC(drpcerr.WithCode(xerrors.New("test"), tc.code))
			require.Equal(t, tc.kind, err.Kind)
		})
	}
}

func TestAuthorizationErrorFromDRPCUnknownCode(t *testing.T) {
	t.Parallel()
	require.Nil(t, authorizationErrorFromDRPC(drpcerr.WithCode(xerrors.New("test"), 9999)))
}

func TestAuthorizationOutcomeForError(t *testing.T) {
	t.Parallel()
	require.Equal(t, intercept.AuthorizationOutcomeDenied, authorizationOutcomeForError(intercept.AuthorizationErrorAuthentication))
	require.Equal(t, intercept.AuthorizationOutcomeDenied, authorizationOutcomeForError(intercept.AuthorizationErrorPolicy))
	require.Equal(t, intercept.AuthorizationOutcomeError, authorizationOutcomeForError(intercept.AuthorizationErrorEvaluation))
}
