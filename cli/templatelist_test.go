package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateList(t *testing.T) {
	t.Parallel()
	t.Run("ListTemplates", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		firstVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, firstVersion.ID)
		firstTemplate := coderdtest.CreateTemplate(t, client, user.OrganizationID, firstVersion.ID)

		secondVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, secondVersion.ID)
		secondTemplate := coderdtest.CreateTemplate(t, client, user.OrganizationID, secondVersion.ID)

		inv, root := clitest.New(t, "templates", "list")
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t).Attach(inv)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		errC := make(chan error)
		go func() {
			errC <- inv.WithContext(ctx).Run()
		}()

		// expect that templates are listed alphabetically
		templatesList := []string{firstTemplate.Name, secondTemplate.Name}
		sort.Strings(templatesList)

		require.NoError(t, <-errC)

		for _, name := range templatesList {
			pty.ExpectMatch(name)
		}
	})
	t.Run("ListTemplatesJSON", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		firstVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, firstVersion.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, firstVersion.ID)

		secondVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, secondVersion.ID)
		_ = coderdtest.CreateTemplate(t, client, user.OrganizationID, secondVersion.ID)

		inv, root := clitest.New(t, "templates", "list", "--output=json")
		clitest.SetupConfig(t, client, root)

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var templates []codersdk.Template
		require.NoError(t, json.Unmarshal(out.Bytes(), &templates))
		require.Len(t, templates, 2)
	})
	t.Run("NoTemplates", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{})
		coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "templates", "list")
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t)
		inv.Stdin = pty.Input()
		inv.Stderr = pty.Output()

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		errC := make(chan error)
		go func() {
			errC <- inv.WithContext(ctx).Run()
		}()

		require.NoError(t, <-errC)

		pty.ExpectMatch("No templates found in")
		pty.ExpectMatch(coderdtest.FirstUserParams.Username)
		pty.ExpectMatch("Create one:")
	})
}
