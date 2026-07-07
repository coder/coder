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

		type baseSpec struct {
			id           string
			expectedOS   string
			expectedVars []string
			hasVariables bool
		}

		specs := []baseSpec{
			{
				id:           "docker",
				expectedOS:   "linux",
				hasVariables: true,
				expectedVars: []string{"container_image"},
			},
			{
				id:           "kubernetes",
				expectedOS:   "linux",
				hasVariables: true,
				expectedVars: []string{"container_image", "namespace", "use_kubeconfig"},
			},
			{
				id:           "aws-linux",
				expectedOS:   "linux",
				hasVariables: false,
			},
			{
				id:           "aws-windows",
				expectedOS:   "windows",
				hasVariables: false,
			},
			{
				id:           "gcp-windows",
				expectedOS:   "windows",
				hasVariables: true,
				expectedVars: []string{"project_id"},
			},
		}

		for _, spec := range specs {
			b, ok := basesByID[spec.id]
			require.True(t, ok, "base %q missing from response", spec.id)
			require.NotEmpty(t, b.Name, "base %q should have a name", spec.id)
			require.NotEmpty(t, b.Icon, "base %q should have an icon", spec.id)
			require.Equal(t, spec.expectedOS, b.OS, "base %q OS mismatch", spec.id)
			require.NotNil(t, b.Variables, "base %q should have non-nil variables slice", spec.id)

			if spec.hasVariables {
				require.NotEmpty(t, b.Variables, "base %q should have variables", spec.id)
				varNames := make(map[string]bool, len(b.Variables))
				for _, v := range b.Variables {
					varNames[v.Name] = true
				}
				for _, expected := range spec.expectedVars {
					require.True(t, varNames[expected],
						"base %q should have variable %q", spec.id, expected)
				}
			} else {
				require.Empty(t, b.Variables, "base %q should have no variables", spec.id)
			}
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
