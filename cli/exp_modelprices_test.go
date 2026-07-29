package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/x/modelprices"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestModelPrices(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	// Start a coderd instance. No provisioner daemon is needed for the
	// model-prices endpoints; the server seeds the embedded price book at
	// startup, so `list` always returns those rows.
	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: false})
	_ = coderdtest.CreateFirstUser(t, client)

	// A small models.dev fixture with two providers and two models. The
	// model IDs are deliberately chosen to not collide with the embedded
	// startup seed so that `update` reports them as additions.
	const fixture = `{
  "anthropic": {
    "models": {
      "test-cli-model-1": {
        "name": "Test CLI Model 1",
        "limit": {"context": 1000000, "output": 128000},
        "cost": {"input": 10, "output": 50, "cache_read": 1, "cache_write": 12.5}
      }
    }
  },
  "openai": {
    "models": {
      "test-cli-model-2": {
        "name": "Test CLI Model 2",
        "limit": {"context": 128000, "output": 16384},
        "cost": {"input": 5, "output": 15}
      }
    }
  }
}`

	// Write the fixture and seed paths into a temp dir.
	tmpDir := t.TempDir()
	fixturePath := filepath.Join(tmpDir, "fixture.json")
	require.NoError(t, os.WriteFile(fixturePath, []byte(fixture), 0o600))
	seedPath := filepath.Join(tmpDir, "seed.json")

	// --- import <fixture> <seed file> ---
	inv, root := clitest.New(t, "exp", "model-prices", "import", fixturePath, seedPath)
	clitest.SetupConfig(t, client, root)
	inv.Stdout = new(bytes.Buffer)
	require.NoError(t, inv.WithContext(ctx).Run(), "import should succeed")

	// The seed file must be valid wire-format JSON produced by Transform.
	seedBytes, err := os.ReadFile(seedPath)
	require.NoError(t, err, "seed file should exist")
	var seedRows []modelprices.PriceRow
	require.NoError(t, json.Unmarshal(seedBytes, &seedRows), "seed file must be valid PriceRow JSON")
	require.Len(t, seedRows, 2, "Transform should produce 2 rows")
	require.Equal(t, "anthropic", seedRows[0].Provider)
	require.Equal(t, "test-cli-model-1", seedRows[0].Model)
	require.Equal(t, "openai", seedRows[1].Provider)
	require.Equal(t, "test-cli-model-2", seedRows[1].Model)

	// --- list --output json (before update) ---
	// The server already seeded the embedded price book at startup, so the
	// list is non-empty. Our test models are not yet present.
	inv, root = clitest.New(t, "exp", "model-prices", "list", "--output", "json")
	clitest.SetupConfig(t, client, root)
	var buf bytes.Buffer
	inv.Stdout = &buf
	require.NoError(t, inv.WithContext(ctx).Run(), "list before update should succeed")
	var beforeUpdate []codersdk.AIModelPrice
	require.NoError(t, json.Unmarshal(buf.Bytes(), &beforeUpdate), "list must emit valid AIModelPrice JSON")
	require.NotEmpty(t, beforeUpdate, "server should have seeded prices at startup")
	for _, p := range beforeUpdate {
		require.NotEqual(t, "test-cli-model-1", p.Model, "test models should not be present before update")
		require.NotEqual(t, "test-cli-model-2", p.Model, "test models should not be present before update")
	}

	// --- update <seed file> --yes ---
	inv, root = clitest.New(t, "exp", "model-prices", "update", seedPath, "--yes")
	clitest.SetupConfig(t, client, root)
	var updateBuf bytes.Buffer
	inv.Stdout = &updateBuf
	require.NoError(t, inv.WithContext(ctx).Run(), "update should succeed")
	require.Contains(t, updateBuf.String(), "Applied", "update output should mention applied")

	// --- list --output json (after update) ---
	// The test models should now be present with the transformed prices.
	inv, root = clitest.New(t, "exp", "model-prices", "list", "--output", "json")
	clitest.SetupConfig(t, client, root)
	var afterBuf bytes.Buffer
	inv.Stdout = &afterBuf
	require.NoError(t, inv.WithContext(ctx).Run(), "list after update should succeed")
	var afterUpdate []codersdk.AIModelPrice
	require.NoError(t, json.Unmarshal(afterBuf.Bytes(), &afterUpdate), "list must emit valid AIModelPrice JSON")

	// Build a lookup so we can assert the imported prices.
	got := map[string]codersdk.AIModelPrice{}
	for _, p := range afterUpdate {
		got[p.Provider+"\x00"+p.Model] = p
	}
	row1, ok1 := got["anthropic\x00test-cli-model-1"]
	require.True(t, ok1, "test-cli-model-1 should be present after update")
	require.NotNil(t, row1.InputPrice, "input price should be set")
	require.Equal(t, int64(10_000_000), *row1.InputPrice, "input price should be $10/MTok in micro-units")
	require.NotNil(t, row1.OutputPrice)
	require.Equal(t, int64(50_000_000), *row1.OutputPrice)
	require.NotNil(t, row1.CacheReadPrice)
	require.Equal(t, int64(1_000_000), *row1.CacheReadPrice)
	require.NotNil(t, row1.CacheWritePrice)
	require.Equal(t, int64(12_500_000), *row1.CacheWritePrice)

	row2, ok2 := got["openai\x00test-cli-model-2"]
	require.True(t, ok2, "test-cli-model-2 should be present after update")
	require.NotNil(t, row2.InputPrice)
	require.Equal(t, int64(5_000_000), *row2.InputPrice)
	require.NotNil(t, row2.OutputPrice)
	require.Equal(t, int64(15_000_000), *row2.OutputPrice)
	require.Nil(t, row2.CacheReadPrice, "cache_read was absent in fixture, should be nil")
	require.Nil(t, row2.CacheWritePrice, "cache_write was absent in fixture, should be nil")

	// --- update <seed file> --yes again (should show "No changes to apply") ---
	inv, root = clitest.New(t, "exp", "model-prices", "update", seedPath, "--yes")
	clitest.SetupConfig(t, client, root)
	var noChangeBuf bytes.Buffer
	inv.Stdout = &noChangeBuf
	require.NoError(t, inv.WithContext(ctx).Run(), "idempotent update should succeed")
	require.Contains(t, strings.ToLower(noChangeBuf.String()), "no changes", "re-running update should report no changes")

	// --- update - --yes (stdin) ---
	inv, root = clitest.New(t, "exp", "model-prices", "update", "-", "--yes")
	clitest.SetupConfig(t, client, root)
	var stdinBuf bytes.Buffer
	inv.Stdout = &stdinBuf
	inv.Stdin = bytes.NewReader(seedBytes)
	require.NoError(t, inv.WithContext(ctx).Run(), "update via stdin should succeed")
	require.Contains(t, strings.ToLower(stdinBuf.String()), "no changes", "stdin update of identical prices should report no changes")

	// --- list (default table format) ---
	// The table output should contain the provider, model, and a formatted
	// price. Table formatting varies, so use strings.Contains.
	inv, root = clitest.New(t, "exp", "model-prices", "list")
	clitest.SetupConfig(t, client, root)
	var tableBuf bytes.Buffer
	inv.Stdout = &tableBuf
	require.NoError(t, inv.WithContext(ctx).Run(), "list (table) should succeed")
	tableOut := tableBuf.String()
	require.Contains(t, tableOut, "anthropic", "table output should contain the anthropic provider")
	require.Contains(t, tableOut, "test-cli-model-1", "table output should contain test-cli-model-1")
	require.Contains(t, tableOut, "$10.00", "table output should contain the formatted input price")

	// --- list --output json --provider anthropic ---
	// All returned rows should be from anthropic; no openai rows.
	inv, root = clitest.New(t, "exp", "model-prices", "list", "--output", "json", "--provider", "anthropic")
	clitest.SetupConfig(t, client, root)
	var provBuf bytes.Buffer
	inv.Stdout = &provBuf
	require.NoError(t, inv.WithContext(ctx).Run(), "list with --provider should succeed")
	var provRows []codersdk.AIModelPrice
	require.NoError(t, json.Unmarshal(provBuf.Bytes(), &provRows), "provider-filtered list must emit valid AIModelPrice JSON")
	require.NotEmpty(t, provRows, "--provider anthropic should return at least one row")
	for _, p := range provRows {
		require.Equal(t, "anthropic", p.Provider, "all rows should be from anthropic")
		require.NotEqual(t, "openai", p.Provider, "no openai rows should be present")
	}
}
