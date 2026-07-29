package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAIModelPrices(t *testing.T) {
	t.Parallel()

	t.Run("PermissionDenied", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		expClient := codersdk.NewExperimentalClient(memberClient)

		// Non-admin GET → 403.
		_, err := expClient.ListAIModelPrices(ctx)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())

		// Non-admin PUT → 403.
		err = expClient.PutAIModelPrices(ctx, []codersdk.AIModelPrice{
			{Provider: "openai", Model: "gpt-4o", InputPrice: ptr.Ref(int64(0))},
		})
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		// Send raw invalid JSON bytes via the underlying client.Request.
		res, err := client.Request(ctx, http.MethodPut,
			"/api/experimental/ai/model-prices",
			[]byte(`{invalid`))
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("EmptyArray", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		res, err := client.Request(ctx, http.MethodPut,
			"/api/experimental/ai/model-prices",
			[]byte(`[]`))
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})
}
