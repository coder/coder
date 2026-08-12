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
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

const aiModelPricesDocument = `[{
	"provider": "anthropic",
	"model": "my-model",
	"input_price": 100,
	"output_price": 200,
	"cache_read_price": null,
	"cache_write_price": null
}]`

// setupAIModelPricesCLI returns a client entitled to manage model prices.
func setupAIModelPricesCLI(t *testing.T) *codersdk.Client {
	t.Helper()

	client, _ := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
				codersdk.FeatureAIBridge:     1,
			},
		},
	})
	return client
}

func TestAIModelPricesUpdate(t *testing.T) {
	t.Parallel()

	// The input modes are rejected before any price is written, so one
	// deployment serves every case.
	t.Run("RejectsInvalidInput", func(t *testing.T) {
		t.Parallel()

		client := setupAIModelPricesCLI(t)

		tests := []struct {
			name    string
			args    []string
			stdin   string
			wantErr string
		}{
			{
				name:    "NoDocumentAndNoFlags",
				args:    []string{"exp", "ai-model-prices", "update"},
				wantErr: "no prices given, pass a JSON document or set --provider, --model and the four price flags",
			},
			{
				name:    "ProviderWithoutModel",
				args:    []string{"exp", "ai-model-prices", "update", "--provider", "anthropic"},
				wantErr: "--provider and --model are both required",
			},
			{
				name:    "ModelWithoutProvider",
				args:    []string{"exp", "ai-model-prices", "update", "--model", "my-model"},
				wantErr: "--provider and --model are both required",
			},
			{
				name:    "PriceFlagWithoutProviderAndModel",
				args:    []string{"exp", "ai-model-prices", "update", "--input-price", "100"},
				wantErr: "--provider and --model are both required",
			},
			{
				name: "SomePriceFlags",
				args: []string{
					"exp", "ai-model-prices", "update",
					"--provider", "anthropic", "--model", "my-model",
					"--input-price", "100", "--output-price", "200",
				},
				wantErr: "all price flags are required",
			},
			{
				name: "NoPriceFlags",
				args: []string{
					"exp", "ai-model-prices", "update",
					"--provider", "anthropic", "--model", "my-model",
				},
				wantErr: "all price flags are required",
			},
			{
				name: "NonNumericPrice",
				args: []string{
					"exp", "ai-model-prices", "update",
					"--provider", "anthropic", "--model", "my-model",
					"--input-price", "abc", "--output-price", "null",
					"--cache-read-price", "null", "--cache-write-price", "null",
				},
				wantErr: `want a whole number or 'null', got "abc"`,
			},
			{
				name: "DocumentAndFlags",
				args: []string{
					"exp", "ai-model-prices", "update", "prices.json",
					"--provider", "anthropic", "--model", "my-model",
				},
				wantErr: "not both",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				inv, conf := newCLI(t, tt.args...)
				clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner
				inv.Stdin = strings.NewReader(tt.stdin)

				err := inv.Run()
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
			})
		}
	})

	t.Run("AppliesADocumentFromStdin", func(t *testing.T) {
		t.Parallel()

		// Given: a licensed deployment and a document on stdin, with no --yes.
		// Draining stdin leaves nothing for a prompt to read, so piping a
		// document applies it without confirmation.
		client := setupAIModelPricesCLI(t)
		inv, conf := newCLI(t, "exp", "ai-model-prices", "update")
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stdout bytes.Buffer
		inv.Stdin = strings.NewReader(aiModelPricesDocument)
		inv.Stdout = &stdout

		// When: the command runs.
		require.NoError(t, inv.Run())

		// Then: the addition is planned and applied.
		require.Contains(t, stdout.String(), "Plan: 1 to add.")
		require.Contains(t, stdout.String(), "+ anthropic/my-model")
		require.Contains(t, stdout.String(), "Updated prices for 1 model(s).")

		ctx := testutil.Context(t, testutil.WaitLong)
		prices, err := codersdk.NewExperimentalClient(client).ListAIModelPrices(ctx,
			codersdk.AIModelPricesFilter{Provider: "anthropic", Model: "my-model"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, int64(100), *prices[0].InputPrice)
	})

	t.Run("AppliesADocumentFromAFile", func(t *testing.T) {
		t.Parallel()

		// Given: the document written to disk.
		client := setupAIModelPricesCLI(t)
		path := filepath.Join(t.TempDir(), "prices.json")
		require.NoError(t, os.WriteFile(path, []byte(aiModelPricesDocument), 0o600))

		inv, conf := newCLI(t, "exp", "ai-model-prices", "update", path, "--yes")
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		// When: the command runs against the file.
		require.NoError(t, inv.Run())

		// Then: the price is applied.
		require.Contains(t, stdout.String(), "Updated prices for 1 model(s).")
	})

	t.Run("AppliesASingleModelFromFlags", func(t *testing.T) {
		t.Parallel()

		// Given: a licensed deployment.
		client := setupAIModelPricesCLI(t)
		inv, conf := newCLI(t,
			"exp", "ai-model-prices", "update",
			"--provider", "anthropic", "--model", "flag-model",
			"--input-price", "100", "--output-price", "null",
			"--cache-read-price", "null", "--cache-write-price", "null",
			"--yes",
		)
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		// When: the model is priced through the flags.
		require.NoError(t, inv.Run())
		require.Contains(t, stdout.String(), "Updated prices for 1 model(s).")

		// Then: the null flags are stored as unknown, not zero.
		ctx := testutil.Context(t, testutil.WaitLong)
		prices, err := codersdk.NewExperimentalClient(client).ListAIModelPrices(ctx,
			codersdk.AIModelPricesFilter{Provider: "anthropic", Model: "flag-model"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, int64(100), *prices[0].InputPrice)
		require.Nil(t, prices[0].OutputPrice)
	})

	t.Run("PreviewsAChangedPrice", func(t *testing.T) {
		t.Parallel()

		// Given: anthropic/change-model already priced at $3.00 per mtok.
		client := setupAIModelPricesCLI(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		input := int64(3_000_000)

		//nolint:gocritic // Managing AI model prices is owner-only.
		require.NoError(t, codersdk.NewExperimentalClient(client).UpsertAIModelPrices(ctx,
			codersdk.UpsertAIModelPricesRequest{
				Prices: []codersdk.AIModelPriceUpsert{{
					Provider: "anthropic", Model: "change-model", InputPrice: &input,
				}},
			}))

		inv, conf := newCLI(t,
			"exp", "ai-model-prices", "update",
			"--provider", "anthropic", "--model", "change-model",
			"--input-price", "5000000", "--output-price", "null",
			"--cache-read-price", "null", "--cache-write-price", "null",
			"--yes",
		)
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		// When: the input price is raised to $5.00.
		require.NoError(t, inv.Run())

		// Then: the plan marks it as a change and shows the transition.
		require.Contains(t, stdout.String(), "Plan: 1 to change.")
		require.Contains(t, stdout.String(), "~ anthropic/change-model")
		require.Contains(t, stdout.String(), "input_price")
		require.Contains(t, stdout.String(), "$3.00 -> $5.00")
	})

	t.Run("ReportsNoChangesOnAReapply", func(t *testing.T) {
		t.Parallel()

		// Given: a document that has already been applied.
		client := setupAIModelPricesCLI(t)
		for range 2 {
			inv, conf := newCLI(t, "exp", "ai-model-prices", "update", "--yes")
			clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

			var stdout bytes.Buffer
			inv.Stdin = strings.NewReader(aiModelPricesDocument)
			inv.Stdout = &stdout
			require.NoError(t, inv.Run())

			// Then: the second run finds nothing to do.
			if strings.Contains(stdout.String(), "No changes to apply.") {
				return
			}
		}
		t.Fatal("re-applying the same document should report no changes")
	})

	t.Run("RejectsAModelInThePriceBook", func(t *testing.T) {
		t.Parallel()

		// Given: a model Coder already prices.
		client := setupAIModelPricesCLI(t)
		inv, conf := newCLI(t,
			"exp", "ai-model-prices", "update",
			"--provider", "anthropic", "--model", "claude-mythos-5",
			"--input-price", "100", "--output-price", "null",
			"--cache-read-price", "null", "--cache-write-price", "null",
			"--yes",
		)
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner
		inv.Stdout = &bytes.Buffer{}

		// When: it is priced. Then: the server rejects it.
		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "price book")
	})
}

func TestAIModelPricesList(t *testing.T) {
	t.Parallel()

	t.Run("JSONCarriesRawMicros", func(t *testing.T) {
		t.Parallel()

		// Given: a priced model.
		client := setupAIModelPricesCLI(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		input := int64(3_000_000)

		//nolint:gocritic // Managing AI model prices is owner-only.
		require.NoError(t, codersdk.NewExperimentalClient(client).UpsertAIModelPrices(ctx,
			codersdk.UpsertAIModelPricesRequest{
				Prices: []codersdk.AIModelPriceUpsert{{
					Provider: "anthropic", Model: "json-model", InputPrice: &input,
				}},
			}))

		inv, conf := newCLI(t, "exp", "ai-model-prices", "list",
			"--provider", "anthropic", "--model", "json-model", "--output", "json")
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		// When: the prices are listed as JSON.
		require.NoError(t, inv.Run())

		// Then: the raw micro-units come back, not the table's dollar strings.
		var prices []codersdk.AIModelPrice
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &prices))
		require.Len(t, prices, 1)
		require.Equal(t, int64(3_000_000), *prices[0].InputPrice)
	})

	t.Run("TableRendersDollarsPerMillionTokens", func(t *testing.T) {
		t.Parallel()

		// Given: a priced model.
		client := setupAIModelPricesCLI(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		input := int64(3_000_000)

		//nolint:gocritic // Managing AI model prices is owner-only.
		require.NoError(t, codersdk.NewExperimentalClient(client).UpsertAIModelPrices(ctx,
			codersdk.UpsertAIModelPricesRequest{
				Prices: []codersdk.AIModelPriceUpsert{{
					Provider: "anthropic", Model: "mymodel", InputPrice: &input,
				}},
			}))

		inv, conf := newCLI(t, "exp", "ai-model-prices", "list",
			"--provider", "anthropic", "--model", "mymodel")
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		// When: the prices are listed as a table.
		require.NoError(t, inv.Run())

		// Then: prices are shown in dollars, and unknown ones as a dash.
		require.Contains(t, stdout.String(), "mymodel")
		require.Contains(t, stdout.String(), "$3.00")
		require.Contains(t, stdout.String(), "-")
	})

	t.Run("SaysWhenNothingMatches", func(t *testing.T) {
		t.Parallel()

		// Given: a filter matching no model.
		client := setupAIModelPricesCLI(t)
		inv, conf := newCLI(t, "exp", "ai-model-prices", "list", "--model", "no-such-model")
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stderr bytes.Buffer
		inv.Stdout = &bytes.Buffer{}
		inv.Stderr = &stderr

		// When: the prices are listed.
		require.NoError(t, inv.Run())

		// Then: an empty table says so rather than printing nothing.
		require.Contains(t, stderr.String(), "No model prices found.")
	})
}
