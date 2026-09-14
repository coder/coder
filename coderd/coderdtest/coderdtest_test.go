package coderdtest_test

import (
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestNew(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	_, _ = coderdtest.NewGoogleInstanceIdentity(t, "example", false)
	_, _ = coderdtest.NewAWSInstanceIdentity(t, "an-instance")
}

func TestRandomName(t *testing.T) {
	t.Parallel()

	for range 10 {
		name := coderdtest.RandomName(t)

		require.NotEmpty(t, name, "name should not be empty")
		require.NotContains(t, name, "_", "name should not contain underscores")

		// Should be title cased (e.g., "Happy Einstein").
		words := strings.Split(name, " ")
		require.Len(t, words, 2, "name should have exactly two words")
		for _, word := range words {
			firstRune := []rune(word)[0]
			require.True(t, unicode.IsUpper(firstRune), "word %q should start with uppercase letter", word)
		}
	}
}
