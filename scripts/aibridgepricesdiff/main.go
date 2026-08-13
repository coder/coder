// aibridgepricesdiff renders a human-readable Markdown summary of the
// difference between two AI Bridge price seed files (the prices.json produced
// by aibridgepricesgen).
//
// The weekly price refresh workflow uses it to fill the pull request body, so
// a reviewer sees which models appeared, which disappeared, and which prices
// moved without reading the raw JSON diff. The raw diff remains the source of
// truth; this output is a review aid.
//
// Usage:
//
//	aibridgepricesdiff -old <path> -new <path>
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/xerrors"
)

// priceRow mirrors the seed file schema written by aibridgepricesgen. Pointer
// fields preserve the distinction between "not populated by upstream" (null)
// and "explicitly zero" (0).
type priceRow struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	InputPrice      *int64 `json:"input_price"`
	OutputPrice     *int64 `json:"output_price"`
	CacheReadPrice  *int64 `json:"cache_read_price"`
	CacheWritePrice *int64 `json:"cache_write_price"`
}

// key identifies a row across the two snapshots.
type key struct {
	provider string
	model    string
}

func (r priceRow) key() key {
	return key{provider: r.Provider, model: r.Model}
}

// priceField names one comparable price on a row, paired with an accessor so
// the comparison loop stays data-driven and column order stays stable.
type priceField struct {
	label string
	get   func(priceRow) *int64
}

var priceFields = []priceField{
	{"input", func(r priceRow) *int64 { return r.InputPrice }},
	{"output", func(r priceRow) *int64 { return r.OutputPrice }},
	{"cache read", func(r priceRow) *int64 { return r.CacheReadPrice }},
	{"cache write", func(r priceRow) *int64 { return r.CacheWritePrice }},
}

// change records a single price field that differs between snapshots.
type change struct {
	provider string
	model    string
	field    string
	old      *int64
	new      *int64
}

// diff is the full comparison between two snapshots.
type diff struct {
	added   []priceRow
	removed []priceRow
	changed []change
}

func (d diff) empty() bool {
	return len(d.added) == 0 && len(d.removed) == 0 && len(d.changed) == 0
}

func main() {
	oldPath := flag.String("old", "", "path to the previous prices.json (required)")
	newPath := flag.String("new", "", "path to the refreshed prices.json (required)")
	flag.Parse()
	if err := run(*oldPath, *newPath, os.Stdout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "aibridgepricesdiff: %v\n", err)
		os.Exit(1)
	}
}

func run(oldPath, newPath string, w io.Writer) error {
	if oldPath == "" || newPath == "" {
		return xerrors.New("-old and -new are both required")
	}
	oldRows, err := readRows(oldPath)
	if err != nil {
		return xerrors.Errorf("read %s: %w", oldPath, err)
	}
	newRows, err := readRows(newPath)
	if err != nil {
		return xerrors.Errorf("read %s: %w", newPath, err)
	}
	_, err = io.WriteString(w, render(compare(oldRows, newRows)))
	return err
}

func readRows(path string) ([]priceRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []priceRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, xerrors.Errorf("parse: %w", err)
	}
	return rows, nil
}

// compare classifies every row as added, removed, or changed. Results are
// sorted by (provider, model) so the same inputs always render identically.
func compare(oldRows, newRows []priceRow) diff {
	oldByKey := make(map[key]priceRow, len(oldRows))
	for _, r := range oldRows {
		oldByKey[r.key()] = r
	}
	newByKey := make(map[key]priceRow, len(newRows))
	for _, r := range newRows {
		newByKey[r.key()] = r
	}

	var d diff
	for _, r := range newRows {
		prev, ok := oldByKey[r.key()]
		if !ok {
			d.added = append(d.added, r)
			continue
		}
		for _, f := range priceFields {
			before, after := f.get(prev), f.get(r)
			if equalPrice(before, after) {
				continue
			}
			d.changed = append(d.changed, change{
				provider: r.Provider,
				model:    r.Model,
				field:    f.label,
				old:      before,
				new:      after,
			})
		}
	}
	for _, r := range oldRows {
		if _, ok := newByKey[r.key()]; !ok {
			d.removed = append(d.removed, r)
		}
	}

	sort.Slice(d.added, func(i, j int) bool { return lessRow(d.added[i], d.added[j]) })
	sort.Slice(d.removed, func(i, j int) bool { return lessRow(d.removed[i], d.removed[j]) })
	sort.SliceStable(d.changed, func(i, j int) bool {
		if d.changed[i].provider != d.changed[j].provider {
			return d.changed[i].provider < d.changed[j].provider
		}
		return d.changed[i].model < d.changed[j].model
	})
	return d
}

func lessRow(a, b priceRow) bool {
	if a.Provider != b.Provider {
		return a.Provider < b.Provider
	}
	return a.Model < b.Model
}

func equalPrice(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// render writes the Markdown summary. Sections with no entries are omitted so
// a small refresh produces a short body.
func render(d diff) string {
	var b strings.Builder
	write := func(format string, args ...any) {
		_, _ = fmt.Fprintf(&b, format, args...)
	}

	write("## Price book changes\n\n")
	if d.empty() {
		write("No price changes; only non-price fields differ.\n")
		return b.String()
	}

	write("%s added, %s removed, %s changed.\n",
		plural(len(d.added), "model"),
		plural(len(d.removed), "model"),
		plural(len(d.changed), "price"),
	)
	write("\nPrices are USD per million tokens.\n")

	renderRows := func(heading string, rows []priceRow) {
		if len(rows) == 0 {
			return
		}
		write("\n### %s\n\n", heading)
		write("| Provider | Model | Input | Output | Cache read | Cache write |\n")
		write("| --- | --- | --- | --- | --- | --- |\n")
		for _, r := range rows {
			write("| %s | %s | %s | %s | %s | %s |\n",
				r.Provider, r.Model,
				formatPrice(r.InputPrice), formatPrice(r.OutputPrice),
				formatPrice(r.CacheReadPrice), formatPrice(r.CacheWritePrice),
			)
		}
	}
	renderRows("Added", d.added)
	renderRows("Removed", d.removed)

	if len(d.changed) > 0 {
		write("\n### Changed\n\n")
		write("| Provider | Model | Field | Old | New | Delta |\n")
		write("| --- | --- | --- | --- | --- | --- |\n")
		for _, c := range d.changed {
			write("| %s | %s | %s | %s | %s | %s |\n",
				c.provider, c.model, c.field,
				formatPrice(c.old), formatPrice(c.new), formatDelta(c.old, c.new),
			)
		}
	}
	return b.String()
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// formatPrice converts integer micro-units back to the upstream USD figure.
// Trailing zeros are trimmed so 10000000 reads as "10" rather than "10.000000".
func formatPrice(micros *int64) string {
	if micros == nil {
		return "unset"
	}
	s := fmt.Sprintf("%.6f", float64(*micros)/1_000_000)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// formatDelta renders the relative move between two prices. A percentage is
// only meaningful when the previous value exists and is non-zero; every other
// transition is described in words.
func formatDelta(prev, next *int64) string {
	switch {
	case prev == nil && next == nil:
		return "n/a"
	case prev == nil:
		return "newly priced"
	case next == nil:
		return "price removed"
	case *prev == 0:
		return "was free"
	}
	pct := (float64(*next) - float64(*prev)) / float64(*prev) * 100
	return fmt.Sprintf("%+.1f%%", pct)
}
