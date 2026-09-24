package intercept_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/intercept"
)

func TestRequestAuthorizer(t *testing.T) {
	t.Parallel()

	var gotProvider, gotModel string
	wantErr := xerrors.New("denied")
	ctx := intercept.WithRequestAuthorizer(context.Background(), func(_ context.Context, providerName, model string) error {
		gotProvider = providerName
		gotModel = model
		return wantErr
	})

	authorizer := intercept.RequestAuthorizerFromContext(ctx)
	require.ErrorIs(t, authorizer(ctx, "bedrock-prod", "arn:aws:bedrock:profile"), wantErr)
	require.Equal(t, "bedrock-prod", gotProvider)
	require.Equal(t, "arn:aws:bedrock:profile", gotModel)
}
