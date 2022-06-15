package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
)

func TestTTL(t *testing.T) {
	t.Parallel()

	t.Run("ShowOK", func(t *testing.T) {
		t.Parallel()

		var (
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			template  = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			ttl       = 7*time.Hour + 30*time.Minute + 30*time.Second
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.TTLMillis = ptr.Ref(ttl.Milliseconds())
			})
			cmdArgs   = []string{"ttl", "show", workspace.Name}
			stdoutBuf = &bytes.Buffer{}
		)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		err := cmd.Execute()
		require.NoError(t, err, "unexpected error")
		require.Equal(t, ttl.Truncate(time.Minute).String(), strings.TrimSpace(stdoutBuf.String()))
	})

	t.Run("UnsetOK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			ttl       = 8*time.Hour + 30*time.Minute + 30*time.Second
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.TTLMillis = ptr.Ref(ttl.Milliseconds())
			})
			cmdArgs   = []string{"ttl", "unset", workspace.Name}
			stdoutBuf = &bytes.Buffer{}
		)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		err := cmd.Execute()
		require.NoError(t, err, "unexpected error")

		// Ensure ttl unset
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Nil(t, updated.TTLMillis, "expected ttl to not be set")
	})

	t.Run("SetOK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			ttl       = 8*time.Hour + 30*time.Minute + 30*time.Second
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.TTLMillis = ptr.Ref(ttl.Milliseconds())
			})
			_       = coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
			cmdArgs = []string{"ttl", "set", workspace.Name, ttl.String()}
			done    = make(chan struct{})
		)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		go func() {
			defer close(done)
			err := cmd.Execute()
			assert.NoError(t, err, "unexpected error")
		}()

		// Ensure ttl updated
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Equal(t, ttl.Truncate(time.Minute), time.Duration(*updated.TTLMillis)*time.Millisecond)

		<-done
	})

	t.Run("ZeroInvalid", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
			ttl       = 8*time.Hour + 30*time.Minute + 30*time.Second
			cmdArgs   = []string{"ttl", "set", workspace.Name, ttl.String()}
			stdoutBuf = &bytes.Buffer{}
		)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		err := cmd.Execute()
		require.NoError(t, err, "unexpected error")

		// Ensure ttl updated
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Equal(t, ttl.Truncate(time.Minute), time.Duration(*updated.TTLMillis)*time.Millisecond)
		require.Contains(t, stdoutBuf.String(), "warning: ttl rounded down")

		// A TTL of zero is not considered valid.
		stdoutBuf.Reset()
		cmd, root = clitest.New(t, "ttl", "set", workspace.Name, "0s")
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		err = cmd.Execute()
		require.EqualError(t, err, "ttl must be at least 1m", "unexpected error")

		// Ensure ttl remains as before
		updated, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Equal(t, ttl.Truncate(time.Minute), time.Duration(*updated.TTLMillis)*time.Millisecond)
	})

	t.Run("Set_NotFound", func(t *testing.T) {
		t.Parallel()

		var (
			client  = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user    = coderdtest.CreateFirstUser(t, client)
			version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_       = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		)

		cmd, root := clitest.New(t, "ttl", "set", "doesnotexist", "8h30m")
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.ErrorContains(t, err, "status code 404:", "unexpected error")
	})

	t.Run("Unset_NotFound", func(t *testing.T) {
		t.Parallel()

		var (
			client  = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user    = coderdtest.CreateFirstUser(t, client)
			version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_       = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		)

		cmd, root := clitest.New(t, "ttl", "unset", "doesnotexist")
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.ErrorContains(t, err, "status code 404:", "unexpected error")
	})

	t.Run("TemplateMaxTTL", func(t *testing.T) {
		t.Parallel()

		var (
			ctx     = context.Background()
			client  = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user    = coderdtest.CreateFirstUser(t, client)
			version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_       = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.MaxTTLMillis = ptr.Ref((8 * time.Hour).Milliseconds())
			})
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
				cwr.TTLMillis = ptr.Ref((8 * time.Hour).Milliseconds())
			})
			cmdArgs   = []string{"ttl", "set", workspace.Name, "24h"}
			stdoutBuf = &bytes.Buffer{}
		)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		err := cmd.Execute()
		require.ErrorContains(t, err, "ttl_ms: ttl must be below template maximum 8h0m0s")

		// Ensure ttl not updated
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.NotNil(t, updated.TTLMillis)
		require.Equal(t, (8 * time.Hour).Milliseconds(), *updated.TTLMillis)
	})
}
