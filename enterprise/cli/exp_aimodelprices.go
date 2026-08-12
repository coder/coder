package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/mattn/go-isatty"
	"golang.org/x/xerrors"

	agplcli "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) aiModelPricesCommand() *serpent.Command {
	return &serpent.Command{
		Use:   "ai-model-prices",
		Short: "Manage AI Governance model prices",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.aiModelPricesList(),
			r.aiModelPricesUpdate(),
		},
	}
}

const modelPricesUpdateDescriptionLong = `Sets prices for models that Coder's price book does not cover. Models the
price book covers cannot be changed.

The JSON document is an array of model prices, in the same shape as
Coder's price book:
  [
    {
      "provider": "anthropic",
      "model": "my-model",
      "input_price": 3000000,
      "output_price": 15000000,
      "cache_read_price": 300000,
      "cache_write_price": null
    }
  ]
  * Prices are micro-units per million tokens, so 3000000 is $3.00 per
    million tokens.
  * A 'null' price is unknown and adds no cost. An explicit 0 declares the
    model free.
  * Every entry sets all four prices, so all four are required. Use 'null'
    for a price you do not have.
`

// aiModelPriceRow renders prices as dollars per million tokens for the table
// output. The embedded price carries the JSON output, keeping the raw
// micro-units for scripting.
type aiModelPriceRow struct {
	// For JSON format:
	codersdk.AIModelPrice `table:"-"`

	// For table format:
	Provider        string `json:"-" table:"provider,default_sort"`
	Model           string `json:"-" table:"model"`
	InputPrice      string `json:"-" table:"input $/mtok"`
	OutputPrice     string `json:"-" table:"output $/mtok"`
	CacheReadPrice  string `json:"-" table:"cache read $/mtok"`
	CacheWritePrice string `json:"-" table:"cache write $/mtok"`
	CreatedAt       string `json:"-" table:"created at"`
	UpdatedAt       string `json:"-" table:"updated at"`
}

func (r *RootCmd) aiModelPricesList() *serpent.Command {
	var (
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
		Short: "List AI Governance model prices",
		Long: "Lists every model priced for this deployment. Narrow the output with " +
			"--provider or --model.",
		Middleware: serpent.Chain(serpent.RequireNArgs(0)),
		Options: serpent.OptionSet{
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

			prices, err := codersdk.NewExperimentalClient(client).ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{
				Provider: provider,
				Model:    model,
			})
			if err != nil {
				return xerrors.Errorf("list model prices: %w", err)
			}

			rows := make([]aiModelPriceRow, 0, len(prices))
			for _, price := range prices {
				rows = append(rows, aiModelPriceRow{
					AIModelPrice:    price,
					Provider:        price.Provider,
					Model:           price.Model,
					InputPrice:      formatMicros(price.InputPrice),
					OutputPrice:     formatMicros(price.OutputPrice),
					CacheReadPrice:  formatMicros(price.CacheReadPrice),
					CacheWritePrice: formatMicros(price.CacheWritePrice),
					CreatedAt:       humanize.Time(price.CreatedAt),
					UpdatedAt:       humanize.Time(price.UpdatedAt),
				})
			}

			// JSON output keeps the empty array so scripts can parse it.
			if len(rows) == 0 && formatter.FormatID() == "table" {
				cliui.Infof(inv.Stderr, "No model prices found.")
				return nil
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
		input      string
		output     string
		cacheRead  string
		cacheWrite string
	)

	cmd := &serpent.Command{
		Use:   "update [file|-]",
		Short: "Set AI Governance model prices",
		Long: modelPricesUpdateDescriptionLong + "\n" + agplcli.FormatExamples(
			agplcli.Example{
				Description: "Set prices for several models from a JSON document.",
				Command:     "coder exp ai-model-prices update prices.json",
			},
			agplcli.Example{
				Description: "Read the document from stdin, applied without confirmation.",
				Command:     "coder exp ai-model-prices update < prices.json",
			},
			agplcli.Example{
				Description: "Set prices for a single model.",
				Command: "coder exp ai-model-prices update --provider anthropic --model my-model " +
					"--input-price 3000000 --output-price 15000000 --cache-read-price 300000 --cache-write-price null",
			},
			agplcli.Example{
				Description: "Set prices without confirmation.",
				Command:     "coder exp ai-model-prices update prices.json --yes",
			},
		),
		Middleware: serpent.Chain(serpent.RequireRangeArgs(0, 1)),
		Options: serpent.OptionSet{
			{
				Flag:        "provider",
				Description: "Provider of the model to price.",
				Value:       serpent.StringOf(&provider),
			},
			{
				Flag:        "model",
				Description: "Model to price. Requires --provider.",
				Value:       serpent.StringOf(&model),
			},
			{
				Flag:        "input-price",
				Description: "Input price in micro-units per million tokens, or 'null' if unknown.",
				Value:       serpent.StringOf(&input),
			},
			{
				Flag:        "output-price",
				Description: "Output price in micro-units per million tokens, or 'null' if unknown.",
				Value:       serpent.StringOf(&output),
			},
			{
				Flag:        "cache-read-price",
				Description: "Cache read price in micro-units per million tokens, or 'null' if unknown.",
				Value:       serpent.StringOf(&cacheRead),
			},
			{
				Flag:        "cache-write-price",
				Description: "Cache write price in micro-units per million tokens, or 'null' if unknown.",
				Value:       serpent.StringOf(&cacheWrite),
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

			requested, fromStdin, err := readAIModelPrices(inv, provider, model)
			if err != nil {
				return err
			}

			current, err := exp.ListAIModelPrices(ctx, codersdk.AIModelPricesFilter{})
			if err != nil {
				return xerrors.Errorf("list model prices: %w", err)
			}
			additions, changes := diffAIModelPrices(requested, current)
			if len(additions) == 0 && len(changes) == 0 {
				_, err = fmt.Fprintln(inv.Stdout, "No changes to apply.")
				return err
			}
			printAIModelPriceChanges(inv, additions, changes)

			// Avoid prompting when the document came from stdin (already drained).
			if !fromStdin {
				if _, err := cliui.Prompt(inv, cliui.PromptOptions{
					Text:      "Apply?",
					IsConfirm: true,
					Default:   cliui.ConfirmNo,
				}); err != nil {
					return err
				}
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

var modelPriceFlags = []string{"input-price", "output-price", "cache-read-price", "cache-write-price"}

// readAIModelPrices picks the input source and returns the requested prices,
// reporting whether they were read from stdin. The single-model flags win when
// any of them is set, a JSON document is read otherwise, and supplying both is
// rejected rather than guessing which one the caller meant. Field-level rules
// are enforced by the server.
func readAIModelPrices(inv *serpent.Invocation, provider, model string) ([]codersdk.AIModelPriceUpsert, bool, error) {
	providedFlags := userSetPriceFlags(inv)
	if provider == "" && model == "" && len(providedFlags) == 0 {
		return readAIModelPricesDocument(inv)
	}
	if len(inv.Args) > 0 {
		return nil, false, xerrors.New("pass either a JSON document or the single-model flags, not both")
	}
	if err := validateModelPriceFlags(provider, model, providedFlags); err != nil {
		return nil, false, err
	}
	price, err := modelPriceFromFlags(inv, provider, model)
	if err != nil {
		return nil, false, err
	}
	return []codersdk.AIModelPriceUpsert{price}, false, nil
}

// userSetPriceFlags lists the price flags the caller supplied.
func userSetPriceFlags(inv *serpent.Invocation) []string {
	var provided []string
	for _, name := range modelPriceFlags {
		if opt := inv.Command.Options.ByFlag(name); opt != nil && opt.ValueSource != serpent.ValueSourceNone {
			provided = append(provided, name)
		}
	}
	return provided
}

// validateModelPriceFlags checks the flag combination names one complete model.
func validateModelPriceFlags(provider, model string, providedFlags []string) error {
	if provider == "" || model == "" {
		return xerrors.New("--provider and --model are both required to price a single model")
	}
	// An entry sets all four prices, so every flag is required. Leaving one out
	// would clear that price rather than preserve it.
	if len(providedFlags) != len(modelPriceFlags) {
		return xerrors.Errorf("all price flags are required: --%s. Pass 'null' for a price you do not have",
			strings.Join(modelPriceFlags, ", --"))
	}
	return nil
}

// modelPriceFromFlags builds the entry named by the single-model flags.
func modelPriceFromFlags(inv *serpent.Invocation, provider, model string) (codersdk.AIModelPriceUpsert, error) {
	price := codersdk.AIModelPriceUpsert{Provider: provider, Model: model}
	for _, name := range modelPriceFlags {
		// A nil value is an unknown price, spelled "null" to match the JSON
		// document and the stored column.
		raw := inv.Command.Options.ByFlag(name).Value.String()
		var value *int64
		if !strings.EqualFold(raw, "null") {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return codersdk.AIModelPriceUpsert{}, xerrors.Errorf("--%s: want a whole number or 'null', got %q", name, raw)
			}
			value = &parsed
		}
		switch name {
		case "input-price":
			price.InputPrice = value
		case "output-price":
			price.OutputPrice = value
		case "cache-read-price":
			price.CacheReadPrice = value
		case "cache-write-price":
			price.CacheWritePrice = value
		}
	}
	return price, nil
}

// isTerminalStdin reports whether stdin is an interactive terminal, meaning no
// document was piped or redirected in.
func isTerminalStdin(inv *serpent.Invocation) bool {
	file, ok := inv.Stdin.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(file.Fd())
}

// readAIModelPricesDocument reads the JSON document from the named file, or
// from stdin when the argument is absent or "-", and reports which of the two
// it read. Draining stdin leaves nothing behind for a prompt to read.
func readAIModelPricesDocument(inv *serpent.Invocation) ([]codersdk.AIModelPriceUpsert, bool, error) {
	var (
		data      []byte
		err       error
		fromStdin bool
	)
	if len(inv.Args) == 0 || inv.Args[0] == "-" {
		// Reading a terminal would block until the caller sends EOF, so leave
		// the document empty rather than appearing to hang.
		if !isTerminalStdin(inv) {
			fromStdin = true
			data, err = io.ReadAll(inv.Stdin)
			if err != nil {
				return nil, false, xerrors.Errorf("read stdin: %w", err)
			}
		}
	} else {
		data, err = os.ReadFile(inv.Args[0])
		if err != nil {
			return nil, false, xerrors.Errorf("read %s: %w", inv.Args[0], err)
		}
	}
	if len(data) == 0 {
		return nil, false, xerrors.New("no prices given, pass a JSON document or set --provider, --model and the four price flags")
	}
	var requested []codersdk.AIModelPriceUpsert
	if err := json.Unmarshal(data, &requested); err != nil {
		return nil, false, xerrors.Errorf("parse prices: %w", err)
	}
	return requested, fromStdin, nil
}

// aiModelPriceChange is a requested price alongside the one it replaces.
type aiModelPriceChange struct {
	price codersdk.AIModelPriceUpsert
	old   codersdk.AIModelPrice
}

// diffAIModelPrices splits the requested prices into models the deployment has
// no price for and models whose prices would move. Requests that match the
// stored prices exactly are dropped.
func diffAIModelPrices(requested []codersdk.AIModelPriceUpsert, current []codersdk.AIModelPrice) ([]codersdk.AIModelPriceUpsert, []aiModelPriceChange) {
	stored := make(map[string]codersdk.AIModelPrice, len(current))
	for _, row := range current {
		stored[row.Provider+"/"+row.Model] = row
	}

	var (
		additions []codersdk.AIModelPriceUpsert
		changes   []aiModelPriceChange
	)
	for _, entry := range requested {
		old, ok := stored[entry.Provider+"/"+entry.Model]
		if !ok {
			additions = append(additions, entry)
			continue
		}
		if pricesEqual(entry, old) {
			continue
		}
		changes = append(changes, aiModelPriceChange{price: entry, old: old})
	}
	return additions, changes
}

// pricesEqual reports whether all four prices already hold the requested values.
func pricesEqual(requested codersdk.AIModelPriceUpsert, stored codersdk.AIModelPrice) bool {
	return priceEqual(requested.InputPrice, stored.InputPrice) &&
		priceEqual(requested.OutputPrice, stored.OutputPrice) &&
		priceEqual(requested.CacheReadPrice, stored.CacheReadPrice) &&
		priceEqual(requested.CacheWritePrice, stored.CacheWritePrice)
}

// priceEqual compares two prices, treating unknown as equal only to unknown.
func priceEqual(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// printAIModelPriceChanges previews the requested changes, marking a model the
// deployment does not price yet with "+" and one whose prices change with "~".
// Nothing is written until the plan is applied.
func printAIModelPriceChanges(inv *serpent.Invocation, additions []codersdk.AIModelPriceUpsert, changes []aiModelPriceChange) {
	var summary []string
	if len(additions) > 0 {
		summary = append(summary, fmt.Sprintf("%d to add", len(additions)))
	}
	if len(changes) > 0 {
		summary = append(summary, fmt.Sprintf("%d to change", len(changes)))
	}
	cliui.Infof(inv.Stdout, "Plan: %s.", strings.Join(summary, ", "))

	for _, price := range additions {
		_, _ = fmt.Fprintf(inv.Stdout, "  + %s/%s   %s\n", price.Provider, price.Model, describePrices(price))
	}
	for _, change := range changes {
		_, _ = fmt.Fprintf(inv.Stdout, "  ~ %s/%s\n", change.price.Provider, change.price.Model)
		for _, line := range describePriceChanges(change) {
			_, _ = fmt.Fprintf(inv.Stdout, "      %s\n", line)
		}
	}
}

// describePrices renders an entry as a one-line list of its set prices.
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

// describePriceChanges renders one "old -> new" line per price that moves.
func describePriceChanges(change aiModelPriceChange) []string {
	old := map[string]*int64{
		"input_price":       change.old.InputPrice,
		"output_price":      change.old.OutputPrice,
		"cache_read_price":  change.old.CacheReadPrice,
		"cache_write_price": change.old.CacheWritePrice,
	}
	var lines []string
	for _, named := range namedPrices(change.price) {
		if priceEqual(named.value, old[named.name]) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%-18s %s -> %s", named.name, formatMicros(old[named.name]), formatMicros(named.value)))
	}
	return lines
}

// namedPrice pairs a price with the field name it is reported under.
type namedPrice struct {
	name  string
	value *int64
}

// namedPrices lists an entry's four prices in field order.
func namedPrices(price codersdk.AIModelPriceUpsert) []namedPrice {
	return []namedPrice{
		{"input_price", price.InputPrice},
		{"output_price", price.OutputPrice},
		{"cache_read_price", price.CacheReadPrice},
		{"cache_write_price", price.CacheWritePrice},
	}
}

// formatMicros renders a micro-unit price as dollars per million tokens. An
// unknown price shows as "-", while a zero price shows as $0.00. A price under
// a cent carries enough decimals to stay distinct from another price that also
// rounds to $0.00.
func formatMicros(price *int64) string {
	if price == nil {
		return "-"
	}
	dollars := float64(*price) / 1_000_000
	if *price > 0 && *price < 10_000 {
		return strings.TrimRight(fmt.Sprintf("$%.6f", dollars), "0")
	}
	return fmt.Sprintf("$%.2f", dollars)
}
