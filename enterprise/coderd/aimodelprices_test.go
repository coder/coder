package coderd_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// setupAIModelPricesTest returns a client entitled to manage model prices.
func setupAIModelPricesTest(t *testing.T) (*codersdk.Client, codersdk.CreateFirstUserResponse) {
	t.Helper()

	return coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
				codersdk.FeatureAIBridge:     1,
			},
		},
	})
}

func newAIModelPrice(provider, model string, input int64) codersdk.AIModelPriceUpsert {
	return codersdk.AIModelPriceUpsert{
		Provider:   provider,
		Model:      model,
		InputPrice: &input,
	}
}

func TestUpsertAIModelPrices(t *testing.T) {
	t.Parallel()

	t.Run("LicenseEntitlement", func(t *testing.T) {
		t.Parallel()

		// Given: a deployment without the AI Bridge feature.
		ownerClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		// When: an owner sets a price for anthropic/my-model.
		//nolint:gocritic // Managing AI model prices is owner-only.
		err := codersdk.NewExperimentalClient(ownerClient).UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{
			Prices: []codersdk.AIModelPriceUpsert{newAIModelPrice("anthropic", "my-model", 100)},
		})

		// Then: RequireFeatureMW rejects it as a Premium feature.
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Premium feature")
	})
	t.Run("Forbidden", func(t *testing.T) {
		t.Parallel()

		// Given: a member without ai_model_price:update.
		ownerClient, owner := setupAIModelPricesTest(t)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)

		// When: they set a price for anthropic/my-model.
		err := codersdk.NewExperimentalClient(memberClient).UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{
			Prices: []codersdk.AIModelPriceUpsert{newAIModelPrice("anthropic", "my-model", 100)},
		})

		// Then: the request is forbidden by ai_model_price:update.
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("RejectsAnOversizedBody", func(t *testing.T) {
		t.Parallel()

		// Given: an entitled deployment and a body over the size cap.
		ownerClient, _ := setupAIModelPricesTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		body := fmt.Sprintf(`{"prices":[{"provider":"anthropic","model":%q}]}`,
			strings.Repeat("a", codersdk.MaxAIModelPricesBytes))

		// When: the body is sent.
		//nolint:gocritic // Managing AI model prices is owner-only.
		res, err := ownerClient.Request(ctx, http.MethodPost,
			"/api/experimental/ai/model-prices", json.RawMessage(body))
		require.NoError(t, err)
		defer res.Body.Close()

		// Then: it is rejected before the body is decoded.
		require.Equal(t, http.StatusRequestEntityTooLarge, res.StatusCode)
	})

	t.Run("RejectsMalformedBody", func(t *testing.T) {
		t.Parallel()

		// Given: an entitled deployment.
		ownerClient, _ := setupAIModelPricesTest(t)

		tests := []struct {
			name       string
			body       string
			wantDetail string
		}{
			{
				name:       "NotAnArray",
				body:       `{"prices": "not-an-array"}`,
				wantDetail: "cannot unmarshal string",
			},
			{
				name:       "WrongFieldType",
				body:       `{"prices":[{"provider":"anthropic","model":"my-model","input_price":"abc","output_price":null,"cache_read_price":null,"cache_write_price":null}]}`,
				wantDetail: "input_price",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				// When: the body is sent. json.RawMessage marshals verbatim, so
				// it reaches the handler unchanged.
				//nolint:gocritic // Managing AI model prices is owner-only.
				res, err := ownerClient.Request(ctx, http.MethodPost,
					"/api/experimental/ai/model-prices", json.RawMessage(tt.body))
				require.NoError(t, err)
				defer res.Body.Close()

				// Then: the detail names the problem, since both decodes report
				// the same message.
				require.Equal(t, http.StatusBadRequest, res.StatusCode)
				var sdkErr *codersdk.Error
				require.ErrorAs(t, codersdk.ReadBodyAsError(res), &sdkErr)
				require.Equal(t, "Request body must be valid JSON.", sdkErr.Message)
				require.Contains(t, sdkErr.Detail, tt.wantDetail)
			})
		}
	})

	t.Run("RejectsInvalidPrices", func(t *testing.T) {
		t.Parallel()

		// Given: an entitled deployment.
		ownerClient, _ := setupAIModelPricesTest(t)
		exp := codersdk.NewExperimentalClient(ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		// When: an entry carries an empty model name.
		//nolint:gocritic // Managing AI model prices is owner-only.
		err := exp.UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{
			Prices: []codersdk.AIModelPriceUpsert{newAIModelPrice("anthropic", "", 100)},
		})

		// Then: a 400 comes back naming the fields that failed.
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Equal(t, "Invalid AI model prices.", sdkErr.Message)
		require.NotEmpty(t, sdkErr.Validations)
	})

	// A rejected request must leave the table untouched, so a large document
	// cannot be applied halfway.
	t.Run("WritesNothingWhenAnyEntryIsInvalid", func(t *testing.T) {
		t.Parallel()

		// Given: the prices already stored.
		ownerClient, _ := setupAIModelPricesTest(t)
		exp := codersdk.NewExperimentalClient(ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Managing AI model prices is owner-only.
		before, err := exp.ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{})
		require.NoError(t, err)

		// When: a document holds a valid good-model ahead of an entry with an
		// empty model name.
		err = exp.UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{
			Prices: []codersdk.AIModelPriceUpsert{
				newAIModelPrice("anthropic", "good-model", 100),
				newAIModelPrice("anthropic", "", 100),
			},
		})
		require.Error(t, err)

		// Then: good-model is not written either, so the row count is unchanged.
		after, err := exp.ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{})
		require.NoError(t, err)
		require.Len(t, after, len(before), "no price should have been written")
	})

	t.Run("SetsPrices", func(t *testing.T) {
		t.Parallel()

		// Given: anthropic/my-model, which the price book does not cover.
		ownerClient, _ := setupAIModelPricesTest(t)
		exp := codersdk.NewExperimentalClient(ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		// When: it is priced with an input price only.
		//nolint:gocritic // Managing AI model prices is owner-only.
		require.NoError(t, exp.UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{
			Prices: []codersdk.AIModelPriceUpsert{newAIModelPrice("anthropic", "my-model", 3_000_000)},
		}))

		// Then: the input price is stored and the other three are null.
		prices, err := exp.ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{
			Provider: "anthropic",
			Model:    "my-model",
		})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, int64(3_000_000), *prices[0].InputPrice)
		require.Nil(t, prices[0].OutputPrice)
		require.Nil(t, prices[0].CacheReadPrice)
		require.Nil(t, prices[0].CacheWritePrice)
	})

	t.Run("UpdatesAPriceItSet", func(t *testing.T) {
		t.Parallel()

		// Given: anthropic/my-model priced at 100 through this endpoint.
		ownerClient, _ := setupAIModelPricesTest(t)
		exp := codersdk.NewExperimentalClient(ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Managing AI model prices is owner-only.
		require.NoError(t, exp.UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{
			Prices: []codersdk.AIModelPriceUpsert{newAIModelPrice("anthropic", "my-model", 100)},
		}))

		// When: the same model is priced again at 200.
		require.NoError(t, exp.UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{
			Prices: []codersdk.AIModelPriceUpsert{newAIModelPrice("anthropic", "my-model", 200)},
		}))

		// Then: the row is updated to 200.
		prices, err := exp.ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{
			Provider: "anthropic",
			Model:    "my-model",
		})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, int64(200), *prices[0].InputPrice)
	})

	// anthropic/claude-opus-5 is covered by the seeded price book, so these
	// write over a price Coder ships.
	t.Run("OverridesAPriceBookModel", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name  string
			price codersdk.AIModelPriceUpsert
			want  codersdk.AIModelPrice
		}{
			{
				name: "ReplacesEveryPrice",
				price: codersdk.AIModelPriceUpsert{
					Provider: "anthropic", Model: "claude-opus-5",
					InputPrice:      ptr.Ref(int64(100)),
					OutputPrice:     ptr.Ref(int64(200)),
					CacheReadPrice:  ptr.Ref(int64(300)),
					CacheWritePrice: ptr.Ref(int64(400)),
				},
				want: codersdk.AIModelPrice{
					InputPrice:      ptr.Ref(int64(100)),
					OutputPrice:     ptr.Ref(int64(200)),
					CacheReadPrice:  ptr.Ref(int64(300)),
					CacheWritePrice: ptr.Ref(int64(400)),
				},
			},
			{
				name:  "StoresNullPricesAsNull",
				price: newAIModelPrice("anthropic", "claude-opus-5", 100),
				want: codersdk.AIModelPrice{
					InputPrice:      ptr.Ref(int64(100)),
					OutputPrice:     nil,
					CacheReadPrice:  nil,
					CacheWritePrice: nil,
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				// Given: a deployment holding the book's price for the model.
				ownerClient, _ := setupAIModelPricesTest(t)
				exp := codersdk.NewExperimentalClient(ownerClient)
				ctx := testutil.Context(t, testutil.WaitLong)

				//nolint:gocritic // Managing AI model prices is owner-only.
				seeded, err := exp.ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{
					Provider: "anthropic", Model: "claude-opus-5",
				})
				require.NoError(t, err)
				require.Len(t, seeded, 1)
				require.NotEqual(t, int64(100), *seeded[0].InputPrice)

				// When: it is priced through the endpoint.
				require.NoError(t, exp.UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{
					Prices: []codersdk.AIModelPriceUpsert{tt.price},
				}))

				// Then: the request is the price in effect, and nothing carries
				// over from the book.
				prices, err := exp.ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{
					Provider: "anthropic", Model: "claude-opus-5",
				})
				require.NoError(t, err)
				require.Len(t, prices, 1)
				require.Equal(t, tt.want.InputPrice, prices[0].InputPrice)
				require.Equal(t, tt.want.OutputPrice, prices[0].OutputPrice)
				require.Equal(t, tt.want.CacheReadPrice, prices[0].CacheReadPrice)
				require.Equal(t, tt.want.CacheWritePrice, prices[0].CacheWritePrice)
			})
		}
	})

	t.Run("StoresAModelNameWithASeparator", func(t *testing.T) {
		t.Parallel()

		// Given: an openrouter model whose ID carries a "/".
		ownerClient, _ := setupAIModelPricesTest(t)
		exp := codersdk.NewExperimentalClient(ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		// When: it is priced.
		//nolint:gocritic // Managing AI model prices is owner-only.
		require.NoError(t, exp.UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{
			Prices: []codersdk.AIModelPriceUpsert{
				newAIModelPrice("openrouter", "anthropic/my-model", 100),
			},
		}))

		// Then: the row is stored under the full model name, and filtering on
		// it round-trips through the query parameter.
		prices, err := exp.ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{
			Provider: "openrouter", Model: "anthropic/my-model",
		})
		require.NoError(t, err)
		require.Len(t, prices, 1)
		require.Equal(t, "openrouter", prices[0].Provider)
		require.Equal(t, "anthropic/my-model", prices[0].Model)
		require.Equal(t, int64(100), *prices[0].InputPrice)
	})
}

func TestListAIModelPrices(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsPriceBook", func(t *testing.T) {
		t.Parallel()

		// Given: a deployment seeded with the embedded price book at startup.
		ownerClient, _ := setupAIModelPricesTest(t)
		exp := codersdk.NewExperimentalClient(ownerClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		// When: the prices are listed.
		//nolint:gocritic // Reading AI model prices is owner-only.
		prices, err := exp.ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{})
		require.NoError(t, err)
		require.NotEmpty(t, prices, "the embedded price book is seeded at startup")

		// Then: a model the book covers comes back with all four prices.
		seeded, err := exp.ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{
			Provider: "anthropic", Model: "claude-opus-5",
		})
		require.NoError(t, err)
		require.Len(t, seeded, 1)
		require.Equal(t, int64(5_000_000), *seeded[0].InputPrice)
		require.Equal(t, int64(25_000_000), *seeded[0].OutputPrice)
		require.Equal(t, int64(500_000), *seeded[0].CacheReadPrice)
		require.Equal(t, int64(6_250_000), *seeded[0].CacheWritePrice)
	})

	t.Run("Filters", func(t *testing.T) {
		t.Parallel()

		// Given: two anthropic models, and an openai model sharing a name with
		// one of them.
		ownerClient, _ := setupAIModelPricesTest(t)
		exp := codersdk.NewExperimentalClient(ownerClient)
		setupCtx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Managing AI model prices is owner-only.
		require.NoError(t, exp.UpsertAIModelPrices(setupCtx, codersdk.UpsertAIModelPricesRequest{
			Prices: []codersdk.AIModelPriceUpsert{
				newAIModelPrice("anthropic", "model-a", 1),
				newAIModelPrice("anthropic", "model-b", 2),
				{Provider: "openai", Model: "model-a", InputPrice: ptr.Ref(int64(3))},
			},
		}))

		tests := []struct {
			name   string
			filter codersdk.AIModelPricesFilter
			want   []string
		}{
			{
				name:   "NoFilter",
				filter: codersdk.AIModelPricesFilter{},
				want:   []string{"anthropic/model-a", "anthropic/model-b", "openai/model-a"},
			},
			{
				name:   "ByProvider",
				filter: codersdk.AIModelPricesFilter{Provider: "anthropic"},
				want:   []string{"anthropic/model-a", "anthropic/model-b"},
			},
			{
				name:   "ByModelSpansProviders",
				filter: codersdk.AIModelPricesFilter{Model: "model-a"},
				want:   []string{"anthropic/model-a", "openai/model-a"},
			},
			{
				name:   "ByProviderAndModel",
				filter: codersdk.AIModelPricesFilter{Provider: "anthropic", Model: "model-a"},
				want:   []string{"anthropic/model-a"},
			},
			{
				name:   "UnknownProviderMatchesNothing",
				filter: codersdk.AIModelPricesFilter{Provider: "unknown-provider"},
				want:   nil,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				// When: the prices are listed with the filter.
				prices, err := exp.ListAIModelPrices(ctx, tt.filter)
				require.NoError(t, err)

				// Then: the expected models come back, and nothing outside the
				// filter does.
				got := make([]string, 0, len(prices))
				for _, price := range prices {
					got = append(got, price.Provider+"/"+price.Model)
					if tt.filter.Provider != "" {
						require.Equal(t, tt.filter.Provider, price.Provider)
					}
					if tt.filter.Model != "" {
						require.Equal(t, tt.filter.Model, price.Model)
					}
				}
				if len(tt.want) == 0 {
					require.Empty(t, got)
					return
				}
				require.Subset(t, got, tt.want)
			})
		}
	})

	t.Run("Forbidden", func(t *testing.T) {
		t.Parallel()

		// Given: a member without ai_model_price:read.
		ownerClient, owner := setupAIModelPricesTest(t)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleMember())
		ctx := testutil.Context(t, testutil.WaitLong)

		// When: they list the prices.
		_, err := codersdk.NewExperimentalClient(memberClient).ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{})

		// Then: the request is forbidden by ai_model_price:read.
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})
}
