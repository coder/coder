package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/aibridge/keys"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

var (
	// aiGatewayCreateRe captures the ID and key prefix from create output.
	aiGatewayCreateRe = regexp.MustCompile(`ID: ([0-9a-f-]+), Prefix: (\S+)\)`)

	// aiGatewayKeyRe captures the one-time secret key from create output,
	// asserting it is KeyLength alphanumeric characters.
	aiGatewayKeyRe = regexp.MustCompile(fmt.Sprintf(`(?s)it will not be shown again\.\n\n([0-9A-Za-z]{%d})\n`, keys.KeyLength))
)

// aiGatewayKey holds the values parsed from `keys create` output.
type aiGatewayKey struct {
	name   string
	id     uuid.UUID
	prefix string
}

// runAIGatewayKeys runs `coder ai-gateway keys <args...>` as the given client
// and returns its stdout, stderr, and the run error.
func runAIGatewayKeys(ctx context.Context, t *testing.T, client *codersdk.Client, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	inv, root := newCLI(t, append([]string{"ai-gateway", "keys"}, args...)...)
	clitest.SetupConfig(t, client, root) //nolint:gocritic // tests run CLI operations as the owner
	outBuf, errBuf := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	inv.Stdout = outBuf
	inv.Stderr = errBuf
	err = inv.WithContext(ctx).Run()
	return outBuf.String(), errBuf.String(), err
}

// createAIGatewayKey creates a key and returns its parsed name, ID, and
// prefix, asserting the create output is well-formed.
func createAIGatewayKey(ctx context.Context, t *testing.T, client *codersdk.Client, name string) aiGatewayKey {
	t.Helper()
	stdout, _, err := runAIGatewayKeys(ctx, t, client, "create", name)
	require.NoError(t, err)
	require.Contains(t, stdout, "Successfully created AI Gateway key "+name)

	// The one-time secret key must be rendered as KeyLength alphanumerics.
	require.Len(t, aiGatewayKeyRe.FindStringSubmatch(stdout), 2, "expected secret key in create output")

	matches := aiGatewayCreateRe.FindStringSubmatch(stdout)
	require.Len(t, matches, 3, "expected ID and Prefix in create output")
	id, err := uuid.Parse(matches[1])
	require.NoError(t, err)
	return aiGatewayKey{name: name, id: id, prefix: matches[2]}
}

// listAIGatewayKeys lists keys as JSON and returns the decoded result.
func listAIGatewayKeys(ctx context.Context, t *testing.T, client *codersdk.Client) []codersdk.AIGatewayKey {
	t.Helper()
	stdout, _, err := runAIGatewayKeys(ctx, t, client, "list", "--output=json")
	require.NoError(t, err)
	var listed []codersdk.AIGatewayKey
	require.NoError(t, json.Unmarshal([]byte(stdout), &listed))
	return listed
}

// deleteAIGatewayKey deletes a key by name or ID, asserts success, and
// returns the command stdout.
func deleteAIGatewayKey(ctx context.Context, t *testing.T, client *codersdk.Client, arg string) string {
	t.Helper()
	stdout, _, err := runAIGatewayKeys(ctx, t, client, "delete", "--yes", arg)
	require.NoError(t, err)
	return stdout
}

func TestAIGatewayKeys(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = true
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dv,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAIBridge: 1,
			},
		},
	})
	t.Run("CRUD", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// List returns empty when no keys exist.
		stdout, stderr, err := runAIGatewayKeys(ctx, t, ownerClient, "list")
		require.NoError(t, err)
		require.Empty(t, stdout)
		require.Contains(t, stderr, "No AI Gateway keys found.")

		// Create two keys and capture their names, IDs, and prefixes.
		created := make([]aiGatewayKey, 0, 2)
		for _, name := range []string{"gateway-key-a", "gateway-key-b"} {
			created = append(created, createAIGatewayKey(ctx, t, ownerClient, name))
		}

		// List returns both created keys as JSON. Sort by name so the
		// assertion does not depend on the list ordering.
		listed := listAIGatewayKeys(ctx, t, ownerClient)
		require.Len(t, listed, 2)
		slices.SortFunc(listed, func(a, b codersdk.AIGatewayKey) int {
			return strings.Compare(a.Name, b.Name)
		})
		for i, want := range created {
			require.Equal(t, want.name, listed[i].Name)
			require.Equal(t, want.id, listed[i].ID)
			require.Equal(t, want.prefix, listed[i].KeyPrefix)
		}

		// Default table output renders created keys, guarding against table
		// column and struct tag drift.
		tableOut, _, err := runAIGatewayKeys(ctx, t, ownerClient, "list")
		require.NoError(t, err)
		require.Contains(t, tableOut, "KEY PREFIX")
		for _, key := range created {
			require.Contains(t, tableOut, key.id.String())
			require.Contains(t, tableOut, key.name)
			require.Contains(t, tableOut, key.prefix)
		}

		// Delete the first key by name.
		stdout = deleteAIGatewayKey(ctx, t, ownerClient, created[0].name)
		require.Contains(t, stdout, "Successfully deleted AI Gateway key "+created[0].name)

		// List returns only the remaining key.
		listed = listAIGatewayKeys(ctx, t, ownerClient)
		require.Len(t, listed, 1)
		require.Equal(t, created[1].name, listed[0].Name)

		// Delete the second key by ID.
		stdout = deleteAIGatewayKey(ctx, t, ownerClient, created[1].id.String())
		require.Contains(t, stdout, "Successfully deleted AI Gateway key "+created[1].name)

		// List returns empty after all keys deleted.
		require.Empty(t, listAIGatewayKeys(ctx, t, ownerClient))

		// Delete a non-existent key returns not found.
		_, _, err = runAIGatewayKeys(ctx, t, ownerClient, "delete", "--yes", created[0].name)
		require.ErrorContains(t, err, created[0].name)
		require.ErrorContains(t, err, "not found")

		// Name resolution takes priority over ID. A valid key name can be
		// formatted like a UUID, so create a key whose name is another key's
		// ID and confirm delete resolves by name first.
		first := createAIGatewayKey(ctx, t, ownerClient, "some-key")
		colliding := createAIGatewayKey(ctx, t, ownerClient, first.id.String())
		require.NotEqual(t, first.id, colliding.id)

		// Deleting with the ID-shaped argument deletes the colliding key
		// (matched by name), not the key whose ID matches.
		stdout = deleteAIGatewayKey(ctx, t, ownerClient, colliding.name)
		require.Contains(t, stdout, "ID: "+colliding.id.String())

		listed = listAIGatewayKeys(ctx, t, ownerClient)
		require.Len(t, listed, 1)
		require.Equal(t, first.id, listed[0].ID)
		require.Equal(t, first.name, listed[0].Name)
	})

	t.Run("InvalidKeyName", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		_, _, err := runAIGatewayKeys(ctx, t, ownerClient, "create", strings.Repeat("a", 65))
		require.ErrorContains(t, err, "create AI Gateway key")
		require.ErrorContains(t, err, "Invalid key name")
	})

	t.Run("MemberForbidden", func(t *testing.T) {
		t.Parallel()

		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		testCases := []struct {
			name string
			args []string
		}{
			{
				name: "list",
				args: []string{"list"},
			},
			{
				name: "create",
				args: []string{"create", "member-key"},
			},
			{
				name: "delete",
				args: []string{"delete", "--yes", uuid.NewString()},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				_, _, err := runAIGatewayKeys(ctx, t, memberClient, tc.args...)
				require.Error(t, err)
				require.ErrorContains(t, err, "Forbidden")
			})
		}
	})
}
