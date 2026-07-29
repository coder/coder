package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/x/modelprices"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) modelPricesCommand() *serpent.Command {
	return &serpent.Command{
		Use:   "model-prices",
		Short: "Manage AI Gateway model pricing",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.modelPricesImport(),
			r.modelPricesUpdate(),
			r.modelPricesList(),
		},
	}
}

// modelPricesImport reads a models.dev JSON snapshot, transforms it into the
// wire-format price rows, and writes the result to a file (or stdout). It does
// NOT touch the API; the two-step import/update workflow lets admins inspect
// or munge the wire-format file before applying it.
func (*RootCmd) modelPricesImport() *serpent.Command {
	return &serpent.Command{
		Use:   "import <file | URL> [<seed file>]",
		Short: "Transform a models.dev snapshot into wire-format price rows",
		Long: "Read a models.dev api.json snapshot (from a file path or HTTP(S) URL), " +
			"flatten the provider → model → cost tree into wire-format price rows, and " +
			"write the result as indented JSON to <seed file> (or stdout when omitted or `-`). " +
			"Does not call the API.",
		Middleware: serpent.RequireRangeArgs(1, 2),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			data, err := readSource(ctx, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("read source: %w", err)
			}

			rows, err := modelprices.Transform(data)
			if err != nil {
				return xerrors.Errorf("transform models.dev snapshot: %w", err)
			}

			out, err := json.MarshalIndent(rows, "", "  ")
			if err != nil {
				return xerrors.Errorf("marshal price rows: %w", err)
			}

			if len(inv.Args) < 2 || inv.Args[1] == "-" {
				_, err = fmt.Fprintln(inv.Stdout, string(out))
				if err != nil {
					return err
				}
			} else {
				if err := os.WriteFile(inv.Args[1], out, 0o600); err != nil {
					return xerrors.Errorf("write seed file: %w", err)
				}
			}

			cliui.Infof(inv.Stderr, "Imported %d models.", len(rows))
			return nil
		},
	}
}

// priceCompare holds only the four price fields, used for cmp.Diff in the
// update reconciliation so timestamps don't make every matching row look
// changed.
type priceCompare struct {
	InputPrice      *int64
	OutputPrice     *int64
	CacheReadPrice  *int64
	CacheWritePrice *int64
}

// modelPricesUpdate reads a wire-format seed file (or stdin via `-`), diffs it
// against the current server state, prints a summary, and (after an
// interactive confirm or `--yes`) applies the full seed via PUT. Rows not in
// the seed are left untouched.
func (r *RootCmd) modelPricesUpdate() *serpent.Command {
	return &serpent.Command{
		Use:   "update <seed file | ->",
		Short: "Apply a wire-format price seed to the server",
		Long: "Read a wire-format price seed (the output of `import`), diff it against the " +
			"current server state, print additions, changes, and unchanged counts, then apply " +
			"the full seed via the API. Use `-` to read the seed from stdin.",
		Middleware: serpent.RequireNArgs(1),
		Options: serpent.OptionSet{
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			exp := codersdk.NewExperimentalClient(client)

			// Read the seed.
			var seedData []byte
			if inv.Args[0] == "-" {
				seedData, err = io.ReadAll(inv.Stdin)
			} else {
				seedData, err = os.ReadFile(inv.Args[0])
			}
			if err != nil {
				return xerrors.Errorf("read seed: %w", err)
			}

			var seedRows []modelprices.PriceRow
			if err := json.Unmarshal(seedData, &seedRows); err != nil {
				return xerrors.Errorf("parse seed JSON: %w", err)
			}
			if len(seedRows) == 0 {
				return xerrors.New("seed file is empty")
			}

			// Fetch the current state.
			current, err := exp.ListAIModelPrices(ctx)
			if err != nil {
				return xerrors.Errorf("list current prices: %w", err)
			}

			// Build maps keyed by provider + "\x00" + model.
			seedMap := make(map[string]modelprices.PriceRow, len(seedRows))
			for _, row := range seedRows {
				seedMap[row.Provider+"\x00"+row.Model] = row
			}
			curMap := make(map[string]codersdk.AIModelPrice, len(current))
			for _, row := range current {
				curMap[row.Provider+"\x00"+row.Model] = row
			}

			// Categorize.
			var additions []modelprices.PriceRow
			type changeEntry struct {
				provider string
				model    string
				old      priceCompare
				new      priceCompare
			}
			var changes []changeEntry
			unchanged := 0
			for key, seedRow := range seedMap {
				curRow, ok := curMap[key]
				if !ok {
					additions = append(additions, seedRow)
					continue
				}
				oldCmp := priceCompare{
					InputPrice:      curRow.InputPrice,
					OutputPrice:     curRow.OutputPrice,
					CacheReadPrice:  curRow.CacheReadPrice,
					CacheWritePrice: curRow.CacheWritePrice,
				}
				newCmp := priceCompare{
					InputPrice:      seedRow.InputPrice,
					OutputPrice:     seedRow.OutputPrice,
					CacheReadPrice:  seedRow.CacheReadPrice,
					CacheWritePrice: seedRow.CacheWritePrice,
				}
				if cmp.Equal(oldCmp, newCmp) {
					unchanged++
					continue
				}
				changes = append(changes, changeEntry{
					provider: seedRow.Provider,
					model:    seedRow.Model,
					old:      oldCmp,
					new:      newCmp,
				})
			}

			// Nothing to do.
			if len(additions) == 0 && len(changes) == 0 {
				_, _ = fmt.Fprintln(inv.Stdout, "No changes to apply.")
				return nil
			}

			// Print additions as a table.
			if len(additions) > 0 {
				cliui.Infof(inv.Stdout, "Additions (%d):", len(additions))
				for _, a := range additions {
					_, _ = fmt.Fprintf(inv.Stdout, "  %s/%s\n", a.Provider, a.Model)
				}
			}

			// Print changes with cmp.Diff per row.
			if len(changes) > 0 {
				cliui.Infof(inv.Stdout, "Changes (%d):", len(changes))
				for _, c := range changes {
					diff := cmp.Diff(c.old, c.new)
					_, _ = fmt.Fprintf(inv.Stdout, "  %s/%s:\n%s\n", c.provider, c.model, indentLines(diff, "    "))
				}
			}

			if unchanged > 0 {
				cliui.Infof(inv.Stdout, "Unchanged: %d", unchanged)
			}

			// Confirm (skipped automatically when --yes is set).
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Apply %d additions and %d changes?", len(additions), len(changes)),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return xerrors.Errorf("apply canceled: %w", err)
			}

			// Construct the SDK payload from the seed rows (zero timestamps).
			sdkPrices := make([]codersdk.AIModelPrice, 0, len(seedRows))
			for _, row := range seedRows {
				sdkPrices = append(sdkPrices, codersdk.AIModelPrice{
					Provider:        row.Provider,
					Model:           row.Model,
					InputPrice:      row.InputPrice,
					OutputPrice:     row.OutputPrice,
					CacheReadPrice:  row.CacheReadPrice,
					CacheWritePrice: row.CacheWritePrice,
				})
			}

			if err := exp.PutAIModelPrices(ctx, sdkPrices); err != nil {
				return xerrors.Errorf("apply prices: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Applied %d additions and %d changes.\n", len(additions), len(changes))
			return nil
		},
	}
}

// modelPriceListRow is the dual-format row type for the list command. The
// embedded codersdk.AIModelPrice carries the raw micro-units and timestamps
// for JSON output (and is excluded from the table via `table:"-"`); the
// string fields carry the human-readable $/MTok values for the table (and are
// excluded from JSON via `json:"-"`).
type modelPriceListRow struct {
	// For JSON format: raw micro-units + timestamps.
	codersdk.AIModelPrice `table:"-"`

	// For table format: $/MTok strings.
	Provider        string `json:"-" table:"provider,default_sort"`
	Model           string `json:"-" table:"model"`
	InputPrice      string `json:"-" table:"input $/mtok"`
	OutputPrice     string `json:"-" table:"output $/mtok"`
	CacheReadPrice  string `json:"-" table:"cache read $/mtok"`
	CacheWritePrice string `json:"-" table:"cache write $/mtok"`
}

// modelPricesList fetches all model prices from the server and prints them as
// a table ($/MTok) or JSON (raw micro-units). Optional --provider and --model
// flags filter client-side.
func (r *RootCmd) modelPricesList() *serpent.Command {
	var (
		provider  string
		model     string
		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat([]modelPriceListRow{}, []string{
				"provider", "model", "input $/mtok", "output $/mtok", "cache read $/mtok", "cache write $/mtok",
			}),
			cliui.JSONFormat(),
		)
	)
	cmd := &serpent.Command{
		Use:        "list",
		Short:      "List configured AI model prices",
		Middleware: serpent.RequireNArgs(0),
		Options: serpent.OptionSet{
			{
				Flag:        "provider",
				Description: "Filter to a single provider.",
				Value:       serpent.StringOf(&provider),
			},
			{
				Flag:        "model",
				Description: "Filter to a single model.",
				Value:       serpent.StringOf(&model),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			exp := codersdk.NewExperimentalClient(client)

			prices, err := exp.ListAIModelPrices(ctx)
			if err != nil {
				return xerrors.Errorf("list model prices: %w", err)
			}

			// Filter client-side.
			rows := make([]modelPriceListRow, 0, len(prices))
			for _, p := range prices {
				if provider != "" && p.Provider != provider {
					continue
				}
				if model != "" && p.Model != model {
					continue
				}
				rows = append(rows, modelPriceListRow{
					AIModelPrice:    p,
					Provider:        p.Provider,
					Model:           p.Model,
					InputPrice:      formatMicros(p.InputPrice),
					OutputPrice:     formatMicros(p.OutputPrice),
					CacheReadPrice:  formatMicros(p.CacheReadPrice),
					CacheWritePrice: formatMicros(p.CacheWritePrice),
				})
			}

			if len(rows) == 0 && formatter.FormatID() == "table" {
				cliui.Infof(inv.Stderr, "No model prices found.")
				return nil
			}

			out, err := formatter.Format(ctx, rows)
			if err != nil {
				return xerrors.Errorf("format output: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stdout, out)
			return nil
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

// readSource fetches data from an HTTP(S) URL or reads it from a local file.
func readSource(ctx context.Context, source string) ([]byte, error) {
	u, err := url.Parse(source)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, xerrors.Errorf("build request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, xerrors.Errorf("fetch URL: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, xerrors.Errorf("fetch URL: %s", resp.Status)
		}
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(source)
}

// formatMicros renders a micro-unit price as "$X.XX" per million tokens, or
// "-" for nil (unknown) prices.
func formatMicros(price *int64) string {
	if price == nil {
		return "-"
	}
	return fmt.Sprintf("$%.2f", float64(*price)/1_000_000)
}

// indentLines prefixes each line of s with prefix. Used to indent cmp.Diff output.
func indentLines(s, prefix string) string {
	return prefix + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n"+prefix)
}
