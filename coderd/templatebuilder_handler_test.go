package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/templatebuilder"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateBuilderBases(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.TemplateBuilderBases(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Bases)
		require.Len(t, resp.Bases, len(templatebuilder.BaseTemplateIDs()))

		basesByID := make(map[string]codersdk.TemplateBuilderBase, len(resp.Bases))
		for _, b := range resp.Bases {
			basesByID[b.ID] = b
		}

		for _, id := range templatebuilder.BaseTemplateIDs() {
			b, ok := basesByID[id]
			require.True(t, ok, "base %q missing from response", id)
			require.NotEmpty(t, b.Name)
			require.NotEmpty(t, b.Icon)
			require.Equal(t, string(templatebuilder.BaseTemplateOS(id)), b.OS)
		}
	})

	t.Run("Sorted", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.TemplateBuilderBases(ctx)
		require.NoError(t, err)

		for i := 1; i < len(resp.Bases); i++ {
			require.LessOrEqual(t, resp.Bases[i-1].Name, resp.Bases[i].Name,
				"bases should be sorted by name")
		}
	})

	t.Run("DisabledReturns404", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.TemplateBuilder.Disabled = true

		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateBuilderBases(ctx)
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}
