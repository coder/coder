//go:build !slim

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/prebuilds"
	"github.com/coder/coder/v2/testutil"
)

func Test_getScaletestPrebuildsTemplates(t *testing.T) {
	t.Parallel()

	client, _, _ := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)

	makeTemplate := func(t *testing.T, name string) {
		t.Helper()
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(r *codersdk.CreateTemplateRequest) {
			r.Name = name
		})
	}

	// The real runner uses a small integer suffix (e.g. "0", "1"), keeping the
	// total name within the 32-character limit enforced by NameValid.
	const (
		scaletestPrebuildName = prebuilds.TemplatePrefix + "0"
		prebuildNoScaletest   = "prebuild-other"
		scaletestNoPrebuild   = "scaletest-other"
		unrelatedTemplate     = "unrelated-template"
	)

	makeTemplate(t, scaletestPrebuildName)
	makeTemplate(t, prebuildNoScaletest)
	makeTemplate(t, scaletestNoPrebuild)
	makeTemplate(t, unrelatedTemplate)

	t.Run("NoFilter", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		got, err := getScaletestPrebuildsTemplates(ctx, client, "")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, scaletestPrebuildName, got[0].Name)
	})

	t.Run("MatchingTemplate", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		got, err := getScaletestPrebuildsTemplates(ctx, client, scaletestPrebuildName)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, scaletestPrebuildName, got[0].Name)
	})

	t.Run("NonExistentScaletestTemplate", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		got, err := getScaletestPrebuildsTemplates(ctx, client, prebuilds.TemplatePrefix+"99")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("NonScaletestTemplateReturnsError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		for _, name := range []string{prebuildNoScaletest, scaletestNoPrebuild, unrelatedTemplate} {
			_, err := getScaletestPrebuildsTemplates(ctx, client, name)
			require.Error(t, err, "expected error for template %q", name)
		}
	})
}
