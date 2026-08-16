// aibridgepricesdiff renders a human-readable Markdown summary of the
// difference between two AI Bridge price seed files (the prices.json produced
// by aibridgepricesgen).
//
// The price refresh workflow uses it to fill the pull request body, so
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

// modelKey identifies a model across the two snapshots.
type modelKey struct {
	provider string
	model    string
}

func (r priceRow) key() modelKey {
	return modelKey{provider: r.Provider, model: r.Model}
}

func (k modelKey) String() string {
	return k.provider + "/" + k.model
}

func less(a, b modelKey) bool {
	if a.provider != b.provider {
		return a.provider < b.provider
	}
	return a.model < b.model
}

// diff is the full comparison between two snapshots
type diff struct {
	added   []modelKey
	removed []modelKey
	changed []modelKey
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

// compare classifies every model as added, removed, or changed.
func compare(oldRows, newRows []priceRow) diff {
	oldByKey := make(map[modelKey]priceRow, len(oldRows))
	for _, r := range oldRows {
		oldByKey[r.key()] = r
	}

	var d diff
	seen := make(map[modelKey]struct{}, len(newRows))
	for _, r := range newRows {
		seen[r.key()] = struct{}{}
		prev, ok := oldByKey[r.key()]
		switch {
		case !ok:
			d.added = append(d.added, r.key())
		case !samePrices(prev, r):
			d.changed = append(d.changed, r.key())
		}
	}
	for _, r := range oldRows {
		if _, ok := seen[r.key()]; !ok {
			d.removed = append(d.removed, r.key())
		}
	}

	for _, keys := range [][]modelKey{d.added, d.removed, d.changed} {
		sort.Slice(keys, func(i, j int) bool { return less(keys[i], keys[j]) })
	}
	return d
}

// samePrices reports whether two rows for the same model carry identical
// prices. A price moving to or from null counts as a change.
func samePrices(a, b priceRow) bool {
	return equalPrice(a.InputPrice, b.InputPrice) &&
		equalPrice(a.OutputPrice, b.OutputPrice) &&
		equalPrice(a.CacheReadPrice, b.CacheReadPrice) &&
		equalPrice(a.CacheWritePrice, b.CacheWritePrice)
}

func equalPrice(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// render writes the Markdown summary: counts, then the models in each
// category. Empty categories are omitted.
func render(d diff) string {
	var b strings.Builder
	write := func(format string, args ...any) {
		_, _ = fmt.Fprintf(&b, format, args...)
	}

	write("## Price book changes\n\n")
	if d.empty() {
		write("No price changes.\n")
		return b.String()
	}

	write("%s added, %s removed, %s changed.\n",
		plural(len(d.added), "model"),
		plural(len(d.removed), "model"),
		plural(len(d.changed), "model"),
	)

	renderList := func(heading string, keys []modelKey) {
		if len(keys) == 0 {
			return
		}
		write("\n### %s\n\n", heading)
		for _, k := range keys {
			write("- %s\n", k)
		}
	}
	renderList("Added", d.added)
	renderList("Removed", d.removed)
	renderList("Changed", d.changed)

	return b.String()
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
