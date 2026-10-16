package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/provisionerkey"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

func TestProvisionerKeys(t *testing.T) {
	t.Parallel()

	t.Run("CRUD", func(t *testing.T) {
		t.Parallel()

		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})
		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))

		name := "dont-TEST-me"
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, conf := newCLI(
			t,
			"provisioner", "keys", "create", name, "--tag", "foo=bar", "--tag", "my=way",
		)

		stdout := expecter.NewAttachedToInvocation(t, inv)
		clitest.SetupConfig(t, orgAdminClient, conf)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		line := stdout.ReadLine(ctx)
		require.Contains(t, line, "Successfully created provisioner key")
		require.Contains(t, line, strings.ToLower(name))
		// empty line
		_ = stdout.ReadLine(ctx)
		key := stdout.ReadLine(ctx)
		require.NotEmpty(t, key)
		require.NoError(t, provisionerkey.Validate(key))

		inv, conf = newCLI(
			t,
			"provisioner", "keys", "ls",
		)
		stdout = expecter.NewAttachedToInvocation(t, inv)
		clitest.SetupConfig(t, orgAdminClient, conf)

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		line = stdout.ReadLine(ctx)
		require.Contains(t, line, "NAME")
		require.Contains(t, line, "CREATED AT")
		require.Contains(t, line, "TAGS")
		line = stdout.ReadLine(ctx)
		require.Contains(t, line, strings.ToLower(name))
		require.Contains(t, line, "foo=bar my=way")

		inv, conf = newCLI(
			t,
			"provisioner", "keys", "delete", "-y", name,
		)

		stdout = expecter.NewAttachedToInvocation(t, inv)
		clitest.SetupConfig(t, orgAdminClient, conf)

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		line = stdout.ReadLine(ctx)
		require.Contains(t, line, "Successfully deleted provisioner key")
		require.Contains(t, line, strings.ToLower(name))

		inv, conf = newCLI(
			t,
			"provisioner", "keys", "ls",
		)
		stdout = expecter.NewAttachedToInvocation(t, inv)
		clitest.SetupConfig(t, orgAdminClient, conf)

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		line = stdout.ReadLine(ctx)
		require.Contains(t, line, "No provisioner keys found")
	})

	t.Run("CreateJSON", func(t *testing.T) {
		t.Parallel()

		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})
		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))

		name := "json-key"
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, conf := newCLI(
			t,
			"provisioner", "keys", "create", name, "--tag", "foo=bar", "--output", "json",
		)
		clitest.SetupConfig(t, orgAdminClient, conf)

		var stdout bytes.Buffer
		inv.Stdout = &stdout
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var created struct {
			codersdk.ProvisionerKey
			Key string `json:"key"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &created))
		require.Equal(t, name, created.Name)
		require.NotEmpty(t, created.ID)
		require.Equal(t, owner.OrganizationID, created.OrganizationID)
		require.Equal(t, "bar", created.Tags["foo"])
		require.NoError(t, provisionerkey.Validate(created.Key))
	})

	t.Run("CreateJSONMetadataUnavailable", func(t *testing.T) {
		t.Parallel()

		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})
		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))

		// Fail only the list call, so the key is created but its metadata
		// lookup fails.
		proxy := httputil.NewSingleHostReverseProxy(client.URL)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/provisionerkeys") {
				http.Error(w, "list unavailable", http.StatusInternalServerError)
				return
			}
			proxy.ServeHTTP(w, r)
		}))
		t.Cleanup(srv.Close)
		proxyURL, err := client.URL.Parse(srv.URL)
		require.NoError(t, err)
		proxyClient := codersdk.New(proxyURL, codersdk.WithSessionToken(orgAdminClient.SessionToken()))

		name := "json-key-no-metadata"
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, conf := newCLI(
			t,
			"provisioner", "keys", "create", name, "--tag", "foo=bar", "--output", "json",
		)
		clitest.SetupConfig(t, proxyClient, conf)

		var stdout, stderr bytes.Buffer
		inv.Stdout = &stdout
		inv.Stderr = &stderr
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var created struct {
			codersdk.ProvisionerKey
			Key string `json:"key"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &created))
		require.Equal(t, name, created.Name)
		require.Equal(t, uuid.Nil, created.ID)
		require.Equal(t, "bar", created.Tags["foo"])
		require.NoError(t, provisionerkey.Validate(created.Key))
		require.Contains(t, stderr.String(), "Could not fetch metadata")
	})
}
