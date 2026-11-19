package chatloop

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
)

func TestErrorTypeKeepsTransportResetClassification(t *testing.T) {
	t.Parallel()

	require.Equal(t, "canceled", errorType(context.Canceled))
	reset := errors.Join(chaterror.ErrProviderTransportReset, context.Canceled)
	require.Equal(t, string(chaterror.Classify(reset).Kind), errorType(reset))
	require.NotEqual(t, "canceled", errorType(reset))
}
