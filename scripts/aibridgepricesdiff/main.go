// aibridgepricesdiff renders a human-readable Markdown summary of the
// difference between two AI Bridge price seed files (the prices.json produced
// by aibridgepricesgen).
//
// The weekly price refresh workflow uses it to fill the pull request body, so
// a reviewer sees at a glance which models appeared, which disappeared, and
// which repriced. Exact figures are deliberately left to the pull request
// diff, which is the source of truth.
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

// render writes the Markdown summary: counts, then the models in each
// category. Only provider and model are listed. Exact prices live in the
// pull request diff, so repeating them here would restate what a reviewer
// can already read.
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

	changedModels := changedModelNames(d.changed)
	write("%s added, %s removed, %s changed across %s.\n",
		plural(len(d.added), "model"),
		plural(len(d.removed), "model"),
		plural(len(d.changed), "price"),
		plural(len(changedModels), "model"),
	)

	renderList := func(heading string, models []string) {
		if len(models) == 0 {
			return
		}
		write("\n### %s\n\n", heading)
		for _, m := range models {
			write("- %s\n", m)
		}
	}
	renderList("Added", modelNames(d.added))
	renderList("Removed", modelNames(d.removed))
	renderList("Changed", changedModels)

	return b.String()
}

// modelNames renders rows as "provider/model", preserving input order.
func modelNames(rows []priceRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Provider+"/"+r.Model)
	}
	return out
}

// changedModelNames collapses per-field changes into one entry per model, so
// a model that repriced across every field is listed once.
func changedModelNames(changes []change) []string {
	var (
		seen = make(map[string]struct{}, len(changes))
		out  []string
	)
	for _, c := range changes {
		name := c.provider + "/" + c.model
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
