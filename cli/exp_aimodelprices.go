package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) aiModelPricesCommand() *serpent.Command {
	return &serpent.Command{
		Use:   "ai-model-prices",
		Short: "Manage AI Gateway model prices",
		Long: "Set prices for models AI Gateway does not price out of the box. " +
			"Without a price, a model's token usage is recorded with no cost, so its " +
			"spend is missing from reporting and is not enforced against budgets.",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.aiModelPricesList(),
			r.aiModelPricesUpdate(),
		},
	}
}

// aiModelPriceRow renders a price as dollars per million tokens for the table
// output. The JSON output uses codersdk.AIModelPrice directly, keeping the raw
// micro-units for scripting.
type aiModelPriceRow struct {
	codersdk.AIModelPrice `table:"-"`

	Provider        string `json:"provider" table:"provider,default_sort"`
	Model           string `json:"model" table:"model"`
	InputPrice      string `json:"input_price" table:"input $/mtok"`
	OutputPrice     string `json:"output_price" table:"output $/mtok"`
	CacheReadPrice  string `json:"cache_read_price" table:"cache read $/mtok"`
	CacheWritePrice string `json:"cache_write_price" table:"cache write $/mtok"`
}

func (r *RootCmd) aiModelPricesList() *serpent.Command {
	var (
		all       bool
		provider  string
		model     string
		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat([]aiModelPriceRow{}, []string{
				"provider", "model", "input $/mtok", "output $/mtok", "cache read $/mtok", "cache write $/mtok",
			}),
			cliui.JSONFormat(),
		)
	)

	cmd := &serpent.Command{
		Use:   "list",
		Short: "List AI model prices",
		Long: "Lists models priced for this deployment but not shipped with Coder. " +
			"Pass --all to include the models Coder prices by default.",
		Middleware: serpent.Chain(serpent.RequireNArgs(0)),
		Options: serpent.OptionSet{
			{
				Flag:        "all",
				Description: "Include models priced by Coder's default price book.",
				Value:       serpent.BoolOf(&all),
			},
			{
				Flag:        "provider",
				Description: "Only show models for this provider.",
				Value:       serpent.StringOf(&provider),
			},
			{
				Flag:        "model",
				Description: "Only show this model.",
				Value:       serpent.StringOf(&model),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			prices, err := codersdk.NewExperimentalClient(client).ListAIModelPrices(ctx, codersdk.ListAIModelPricesOptions{
				IncludeDefaults: all,
			})
			if err != nil {
				return xerrors.Errorf("list model prices: %w", err)
			}

			rows := make([]aiModelPriceRow, 0, len(prices))
			for _, price := range prices {
				if provider != "" && price.Provider != provider {
					continue
				}
				if model != "" && price.Model != model {
					continue
				}
				rows = append(rows, aiModelPriceRow{
					AIModelPrice:    price,
					Provider:        price.Provider,
					Model:           price.Model,
					InputPrice:      formatMicros(price.InputPrice),
					OutputPrice:     formatMicros(price.OutputPrice),
					CacheReadPrice:  formatMicros(price.CacheReadPrice),
					CacheWritePrice: formatMicros(price.CacheWritePrice),
				})
			}

			out, err := formatter.Format(ctx, rows)
			if err != nil {
				return xerrors.Errorf("format output: %w", err)
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) aiModelPricesUpdate() *serpent.Command {
	var (
		provider   string
		model      string
		input      int64
		output     int64
		cacheRead  int64
		cacheWrite int64
	)

	cmd := &serpent.Command{
		Use:   "update",
		Short: "Set prices for AI models",
		Long: FormatExamples(
			Example{
				Description: "Set prices for several models from a JSON document.",
				Command:     "coder exp ai-model-prices update < prices.json",
			},
			Example{
				Description: "Set prices for a single model.",
				Command: "coder exp ai-model-prices update --provider anthropic --model my-model " +
					"--input-price 3000000 --output-price 15000000",
			},
		),
		Middleware: serpent.Chain(serpent.RequireNArgs(0)),
		Options: serpent.OptionSet{
			{
				Flag:        "provider",
				Description: "Provider of the model to price. Switches to single-model mode.",
				Value:       serpent.StringOf(&provider),
			},
			{
				Flag:        "model",
				Description: "Model to price. Requires --provider.",
				Value:       serpent.StringOf(&model),
			},
			{
				Flag:        "input-price",
				Description: "Input price in micro-units per million tokens.",
				Value:       serpent.Int64Of(&input),
			},
			{
				Flag:        "output-price",
				Description: "Output price in micro-units per million tokens.",
				Value:       serpent.Int64Of(&output),
			},
			{
				Flag:        "cache-read-price",
				Description: "Cache read price in micro-units per million tokens.",
				Value:       serpent.Int64Of(&cacheRead),
			},
			{
				Flag:        "cache-write-price",
				Description: "Cache write price in micro-units per million tokens.",
				Value:       serpent.Int64Of(&cacheWrite),
			},
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			exp := codersdk.NewExperimentalClient(client)

			requested, err := readAIModelPrices(inv, provider, model)
			if err != nil {
				return err
			}

			// Default-priced models are rejected by the server, so they can
			// never appear in a diff and are not worth fetching.
			current, err := exp.ListAIModelPrices(ctx, codersdk.ListAIModelPricesOptions{})
			if err != nil {
				return xerrors.Errorf("list model prices: %w", err)
			}
			additions, changes := diffAIModelPrices(requested, current)
			if len(additions) == 0 && len(changes) == 0 {
				_, err = fmt.Fprintln(inv.Stdout, "No changes to apply.")
				return err
			}
			printAIModelPriceChanges(inv, additions, changes)

			// Prompt returns immediately when --yes is set.
			if _, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Apply?",
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			}); err != nil {
				return err
			}

			if err := exp.UpsertAIModelPrices(ctx, codersdk.UpsertAIModelPricesRequest{Prices: requested}); err != nil {
				return xerrors.Errorf("update model prices: %w", err)
			}
			_, err = fmt.Fprintf(inv.Stdout, "Updated prices for %d model(s).\n", len(additions)+len(changes))
			return err
		},
	}
	return cmd
}

// readAIModelPrices builds the requested prices from flags when --provider is
// set, and from stdin otherwise. Mixing the two is rejected rather than
// guessing which one the caller meant.
func readAIModelPrices(inv *serpent.Invocation, provider, model string) ([]codersdk.AIModelPriceUpsert, error) {
	priceFlags := []string{"input-price", "output-price", "cache-read-price", "cache-write-price"}
	var setPrices []string
	for _, name := range priceFlags {
		if opt := inv.Command.Options.ByFlag(name); opt != nil && opt.ValueSource != serpent.ValueSourceNone {
			setPrices = append(setPrices, name)
		}
	}
	flagMode := provider != "" || model != "" || len(setPrices) > 0

	if !flagMode {
		return readAIModelPricesStdin(inv)
	}

	if provider == "" || model == "" {
		return nil, xerrors.New("--provider and --model are both required to price a single model")
	}
	if len(setPrices) == 0 {
		return nil, xerrors.New("at least one price flag is required; pass an explicit 0 to declare a model free")
	}

	price := codersdk.AIModelPriceUpsert{Provider: provider, Model: model}
	for _, name := range setPrices {
		opt := inv.Command.Options.ByFlag(name)
		value, err := strconv.ParseInt(opt.Value.String(), 10, 64)
		if err != nil {
			return nil, xerrors.Errorf("parse --%s: %w", name, err)
		}
		switch name {
		case "input-price":
			price.InputPrice = &value
		case "output-price":
			price.OutputPrice = &value
		case "cache-read-price":
			price.CacheReadPrice = &value
		case "cache-write-price":
			price.CacheWritePrice = &value
		}
	}
	return []codersdk.AIModelPriceUpsert{price}, nil
}

func readAIModelPricesStdin(inv *serpent.Invocation) ([]codersdk.AIModelPriceUpsert, error) {
	data, err := io.ReadAll(inv.Stdin)
	if err != nil {
		return nil, xerrors.Errorf("read stdin: %w", err)
	}
	if len(data) == 0 {
		return nil, xerrors.New("no prices given; pipe a JSON document or use --provider and --model")
	}
	var requested []codersdk.AIModelPriceUpsert
	if err := json.Unmarshal(data, &requested); err != nil {
		return nil, xerrors.Errorf("parse prices: %w", err)
	}
	return requested, nil
}

type aiModelPriceChange struct {
	price codersdk.AIModelPriceUpsert
	old   codersdk.AIModelPrice
}

// diffAIModelPrices splits the requested prices into models the deployment has
// no price for and models whose prices would move. Requests that match the
// stored prices exactly are dropped.
func diffAIModelPrices(requested []codersdk.AIModelPriceUpsert, current []codersdk.AIModelPrice) ([]codersdk.AIModelPriceUpsert, []aiModelPriceChange) {
	stored := make(map[string]codersdk.AIModelPrice, len(current))
	for _, price := range current {
		stored[price.Provider+"/"+price.Model] = price
	}

	var (
		additions []codersdk.AIModelPriceUpsert
		changes   []aiModelPriceChange
	)
	for _, price := range requested {
		old, ok := stored[price.Provider+"/"+price.Model]
		if !ok {
			additions = append(additions, price)
			continue
		}
		if samePrices(price, old) {
			continue
		}
		changes = append(changes, aiModelPriceChange{price: price, old: old})
	}
	return additions, changes
}

func samePrices(requested codersdk.AIModelPriceUpsert, stored codersdk.AIModelPrice) bool {
	return samePrice(requested.InputPrice, stored.InputPrice) &&
		samePrice(requested.OutputPrice, stored.OutputPrice) &&
		samePrice(requested.CacheReadPrice, stored.CacheReadPrice) &&
		samePrice(requested.CacheWritePrice, stored.CacheWritePrice)
}

func samePrice(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func printAIModelPriceChanges(inv *serpent.Invocation, additions []codersdk.AIModelPriceUpsert, changes []aiModelPriceChange) {
	if len(additions) > 0 {
		cliui.Infof(inv.Stdout, "Adding %d model(s):", len(additions))
		for _, price := range additions {
			_, _ = fmt.Fprintf(inv.Stdout, "  %s/%s   %s\n", price.Provider, price.Model, describePrices(price))
		}
	}
	if len(changes) > 0 {
		cliui.Infof(inv.Stdout, "Changing %d model(s):", len(changes))
		for _, change := range changes {
			_, _ = fmt.Fprintf(inv.Stdout, "  %s/%s\n", change.price.Provider, change.price.Model)
			for _, line := range describePriceChanges(change) {
				_, _ = fmt.Fprintf(inv.Stdout, "    %s\n", line)
			}
		}
	}
}

func describePrices(price codersdk.AIModelPriceUpsert) string {
	var parts []string
	for _, named := range namedPrices(price) {
		if named.value == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s", named.name, formatMicros(named.value)))
	}
	return strings.Join(parts, "  ")
}

func describePriceChanges(change aiModelPriceChange) []string {
	old := map[string]*int64{
		"input_price":       change.old.InputPrice,
		"output_price":      change.old.OutputPrice,
		"cache_read_price":  change.old.CacheReadPrice,
		"cache_write_price": change.old.CacheWritePrice,
	}
	var lines []string
	for _, named := range namedPrices(change.price) {
		if samePrice(named.value, old[named.name]) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%-18s %s -> %s", named.name, formatMicros(old[named.name]), formatMicros(named.value)))
	}
	return lines
}

type namedPrice struct {
	name  string
	value *int64
}

func namedPrices(price codersdk.AIModelPriceUpsert) []namedPrice {
	return []namedPrice{
		{"input_price", price.InputPrice},
		{"output_price", price.OutputPrice},
		{"cache_read_price", price.CacheReadPrice},
		{"cache_write_price", price.CacheWritePrice},
	}
}

// formatMicros renders a micro-unit price as dollars per million tokens. A nil
// price is shown as "-" to distinguish "not set" from an explicit zero.
func formatMicros(price *int64) string {
	if price == nil {
		return "-"
	}
	return fmt.Sprintf("$%.2f", float64(*price)/1_000_000)
}
