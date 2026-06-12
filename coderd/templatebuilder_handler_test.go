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

func TestTemplateBuilderModules(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.TemplateBuilderModules(ctx, "")
		require.NoError(t, err)
		require.NotEmpty(t, resp.Modules)

		for _, m := range resp.Modules {
			require.NotEmpty(t, m.ID)
			require.NotEmpty(t, m.Version)
		}
	})

	t.Run("FilteredByBase", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.TemplateBuilderModules(ctx, "docker")
		require.NoError(t, err)

		for _, m := range resp.Modules {
			if len(m.CompatibleOS) > 0 {
				require.Contains(t, m.CompatibleOS, "linux",
					"module %q should be compatible with linux when filtered by docker base", m.ID)
			}
		}
	})

	t.Run("ComputedVariablesExcluded", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.TemplateBuilderModules(ctx, "")
		require.NoError(t, err)

		// The embedded code-server module has agent_id with computed=true.
		// It must not appear in the API response.
		var found bool
		for _, m := range resp.Modules {
			if m.ID == "code-server" {
				found = true
				for _, v := range m.Variables {
					require.NotEqual(t, "agent_id", v.Name,
						"computed variable agent_id must not appear in API response")
				}
			}
		}
		require.True(t, found, "code-server module must be in the catalog")
	})

	t.Run("UnknownBaseReturns400", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateBuilderModules(ctx, "nonexistent")
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
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

		_, err := client.TemplateBuilderModules(ctx, "")
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}
