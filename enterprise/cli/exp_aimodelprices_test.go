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

		// Then: every column in the document is stored, nulls included.
		ctx := testutil.Context(t, testutil.WaitLong)
		prices, err := codersdk.NewExperimentalClient(client).ListAIModelPrices(ctx,
			codersdk.AIModelPricesFilter{Provider: "anthropic", Model: "my-model"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, int64(100), *prices[0].InputPrice)
		require.Equal(t, int64(200), *prices[0].OutputPrice)
		require.Nil(t, prices[0].CacheReadPrice)
		require.Nil(t, prices[0].CacheWritePrice)
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
			"--input-price", "100", "--output-price", "200",
			"--cache-read-price", "300", "--cache-write-price", "null",
			"--yes",
		)
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		// When: the model is priced through the flags.
		require.NoError(t, inv.Run())
		require.Contains(t, stdout.String(), "Updated prices for 1 model(s).")

		// Then: each flag lands in its own column, and the null flag is stored
		// as unknown rather than zero.
		ctx := testutil.Context(t, testutil.WaitLong)
		prices, err := codersdk.NewExperimentalClient(client).ListAIModelPrices(ctx,
			codersdk.AIModelPricesFilter{Provider: "anthropic", Model: "flag-model"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, int64(100), *prices[0].InputPrice)
		require.Equal(t, int64(200), *prices[0].OutputPrice)
		require.Equal(t, int64(300), *prices[0].CacheReadPrice)
		require.Nil(t, prices[0].CacheWritePrice)
	})

	t.Run("PreviewsAChangedPrice", func(t *testing.T) {
		t.Parallel()

		// Given: anthropic/change-model priced on all four columns.
		client := setupAIModelPricesCLI(t)
		exp := codersdk.NewExperimentalClient(client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Managing AI model prices is owner-only.
		require.NoError(t, exp.UpsertAIModelPrices(ctx,
			codersdk.UpsertAIModelPricesRequest{
				Prices: []codersdk.AIModelPriceUpsert{{
					Provider: "anthropic", Model: "change-model",
					InputPrice:      new(int64(3_000_000)),
					OutputPrice:     new(int64(15_000_000)),
					CacheReadPrice:  new(int64(300_000)),
					CacheWritePrice: new(int64(1_000_000)),
				}},
			}))

		inv, conf := newCLI(t,
			"exp", "ai-model-prices", "update",
			"--provider", "anthropic", "--model", "change-model",
			"--input-price", "5000000", "--output-price", "16000000",
			"--cache-read-price", "400000", "--cache-write-price", "null",
			"--yes",
		)
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		// When: every price is changed, including one cleared to unknown.
		require.NoError(t, inv.Run())

		// Then: the plan marks it as a change and shows each transition.
		require.Contains(t, stdout.String(), "Plan: 1 to change.")
		require.Contains(t, stdout.String(), "~ anthropic/change-model")
		require.Contains(t, stdout.String(), "$3.00 -> $5.00")
		require.Contains(t, stdout.String(), "$15.00 -> $16.00")
		require.Contains(t, stdout.String(), "$0.30 -> $0.40")
		require.Contains(t, stdout.String(), "$1.00 -> -")

		// Then: every column holds the new price, and the cleared one is unknown.
		prices, err := exp.ListAIModelPrices(ctx,
			codersdk.AIModelPricesFilter{Provider: "anthropic", Model: "change-model"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, int64(5_000_000), *prices[0].InputPrice)
		require.Equal(t, int64(16_000_000), *prices[0].OutputPrice)
		require.Equal(t, int64(400_000), *prices[0].CacheReadPrice)
		require.Nil(t, prices[0].CacheWritePrice)
	})

	t.Run("ReportsNoChangesOnAReapply", func(t *testing.T) {
		t.Parallel()

		client := setupAIModelPricesCLI(t)
		apply := func() string {
			inv, conf := newCLI(t, "exp", "ai-model-prices", "update")
			clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

			var stdout bytes.Buffer
			inv.Stdin = strings.NewReader(aiModelPricesDocument)
			inv.Stdout = &stdout
			require.NoError(t, inv.Run())
			return stdout.String()
		}

		// Given: a document that has already been applied.
		require.Contains(t, apply(), "Updated prices for 1 model(s).")

		// When: the same document is applied again.
		// Then: the diff is empty, so nothing is written.
		require.Contains(t, apply(), "No changes to apply.")
	})

	t.Run("OverridesAModelInThePriceBook", func(t *testing.T) {
		t.Parallel()

		// Given: a model Coder already prices.
		client := setupAIModelPricesCLI(t)
		inv, conf := newCLI(t,
			"exp", "ai-model-prices", "update",
			"--provider", "anthropic", "--model", "claude-opus-5",
			"--input-price", "100", "--output-price", "200",
			"--cache-read-price", "300", "--cache-write-price", "null",
			"--yes",
		)
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		// When: it is priced through the CLI.
		require.NoError(t, inv.Run())
		require.Contains(t, stdout.String(), "Updated prices for 1 model(s).")

		// Then: the override is what the deployment reports.
		ctx := testutil.Context(t, testutil.WaitLong)
		prices, err := codersdk.NewExperimentalClient(client).ListAIModelPrices(ctx,
			codersdk.AIModelPricesFilter{Provider: "anthropic", Model: "claude-opus-5"})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, int64(100), *prices[0].InputPrice)
		require.Equal(t, int64(200), *prices[0].OutputPrice)
		require.Equal(t, int64(300), *prices[0].CacheReadPrice)
		require.Nil(t, prices[0].CacheWritePrice)
	})
}

func TestAIModelPricesList(t *testing.T) {
	t.Parallel()

	t.Run("JSONCarriesRawMicros", func(t *testing.T) {
		t.Parallel()

		// Given: a model priced on three columns, with one left unknown.
		client := setupAIModelPricesCLI(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Managing AI model prices is owner-only.
		require.NoError(t, codersdk.NewExperimentalClient(client).UpsertAIModelPrices(ctx,
			codersdk.UpsertAIModelPricesRequest{
				Prices: []codersdk.AIModelPriceUpsert{{
					Provider: "anthropic", Model: "json-model",
					InputPrice:     new(int64(3_000_000)),
					OutputPrice:    new(int64(15_000_000)),
					CacheReadPrice: new(int64(300_000)),
				}},
			}))

		inv, conf := newCLI(t, "exp", "ai-model-prices", "list",
			"--provider", "anthropic", "--model", "json-model", "--output", "json")
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		// When: the prices are listed as JSON.
		require.NoError(t, inv.Run())

		// Then: every column comes back as raw micro-units, not the table's
		// dollar strings, and the unknown one stays null.
		var prices []codersdk.AIModelPrice
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &prices))
		require.Len(t, prices, 1)
		require.Equal(t, "anthropic", prices[0].Provider)
		require.Equal(t, "json-model", prices[0].Model)
		require.Equal(t, int64(3_000_000), *prices[0].InputPrice)
		require.Equal(t, int64(15_000_000), *prices[0].OutputPrice)
		require.Equal(t, int64(300_000), *prices[0].CacheReadPrice)
		require.Nil(t, prices[0].CacheWritePrice)
	})

	t.Run("TableRendersDollarsPerMillionTokens", func(t *testing.T) {
		t.Parallel()

		// Given: a model priced on three columns, one of them under a cent,
		// with the fourth left unknown.
		client := setupAIModelPricesCLI(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Managing AI model prices is owner-only.
		require.NoError(t, codersdk.NewExperimentalClient(client).UpsertAIModelPrices(ctx,
			codersdk.UpsertAIModelPricesRequest{
				Prices: []codersdk.AIModelPriceUpsert{{
					Provider: "anthropic", Model: "mymodel",
					InputPrice:     new(int64(3_000_000)),
					OutputPrice:    new(int64(15_000_000)),
					CacheReadPrice: new(int64(3_600)),
				}},
			}))

		inv, conf := newCLI(t, "exp", "ai-model-prices", "list",
			"--provider", "anthropic", "--model", "mymodel")
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		// When: the prices are listed as a table.
		require.NoError(t, inv.Run())

		// Then: each column is shown in dollars, a sub-cent price keeps enough
		// decimals to stay distinct, and the unknown one shows as a dash.
		require.Contains(t, stdout.String(), "mymodel")
		require.Contains(t, stdout.String(), "$3.00")
		require.Contains(t, stdout.String(), "$15.00")
		require.Contains(t, stdout.String(), "$0.0036")
		require.Contains(t, stdout.String(), "-")
		require.Contains(t, stdout.String(), "custom")
	})

	t.Run("FiltersBySource", func(t *testing.T) {
		t.Parallel()

		// Given: anthropic/claude-opus-5 priced by the seeded book and then
		// overridden, so it carries a row under each source.
		client := setupAIModelPricesCLI(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		list := func(source string) []codersdk.AIModelPrice {
			inv, conf := newCLI(t, "exp", "ai-model-prices", "list",
				"--provider", "anthropic", "--model", "claude-opus-5",
				"--source", source, "--output", "json")
			clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

			var stdout bytes.Buffer
			inv.Stdout = &stdout
			require.NoError(t, inv.Run())

			var prices []codersdk.AIModelPrice
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &prices))
			return prices
		}

		// The book's row is captured rather than hardcoded, so the assertion
		// survives a price book update.
		seeded := list("default")
		require.Len(t, seeded, 1)

		//nolint:gocritic // Managing AI model prices is owner-only.
		require.NoError(t, codersdk.NewExperimentalClient(client).UpsertAIModelPrices(ctx,
			codersdk.UpsertAIModelPricesRequest{
				Prices: []codersdk.AIModelPriceUpsert{{
					Provider: "anthropic", Model: "claude-opus-5", InputPrice: new(int64(100)),
				}},
			}))

		// When: each source is listed. Then: the model reports under either
		// filter, at the price that source holds.
		def := list("default")
		require.Len(t, def, 1)
		require.Equal(t, codersdk.AIModelPriceSourceDefault, def[0].Source)
		require.Equal(t, seeded[0].InputPrice, def[0].InputPrice)
		require.Equal(t, seeded[0].OutputPrice, def[0].OutputPrice)
		require.Equal(t, seeded[0].CacheReadPrice, def[0].CacheReadPrice)
		require.Equal(t, seeded[0].CacheWritePrice, def[0].CacheWritePrice)

		// The request set an input price only, so the other three are null.
		custom := list("custom")
		require.Len(t, custom, 1)
		require.Equal(t, codersdk.AIModelPriceSourceCustom, custom[0].Source)
		require.Equal(t, int64(100), *custom[0].InputPrice)
		require.Nil(t, custom[0].OutputPrice)
		require.Nil(t, custom[0].CacheReadPrice)
		require.Nil(t, custom[0].CacheWritePrice)

		// The "all" source reports both rows at once, custom first.
		all := list("all")
		require.Len(t, all, 2)
		require.Equal(t, custom[0], all[0])
		require.Equal(t, def[0], all[1])
	})

	t.Run("RejectsAnUnknownSource", func(t *testing.T) {
		t.Parallel()

		client := setupAIModelPricesCLI(t)
		inv, conf := newCLI(t, "exp", "ai-model-prices", "list", "--source", "seeded")
		clitest.SetupConfig(t, client, conf) //nolint:gocritic // requires owner

		// When: an unknown source is passed. Then: the flag rejects it.
		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "seeded")
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
