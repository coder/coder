// aibridgepricesdiff renders a human-readable Markdown summary of the
// difference between two AI Gateway price seed files (the prices.json produced
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
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge/prices/pricebook"
)

func less(a, b pricebook.Key) bool {
	if a.Provider != b.Provider {
		return a.Provider < b.Provider
	}
	return a.Model < b.Model
}

// diff is the full comparison between two snapshots
type diff struct {
	added   []pricebook.Key
	removed []pricebook.Key
	changed []pricebook.Key
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

func readRows(path string) ([]pricebook.Row, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rows, err := pricebook.Parse(data)
	if err != nil {
		return nil, xerrors.Errorf("parse: %w", err)
	}
	return rows, nil
}

// compare classifies every model as added, removed, or changed.
func compare(oldRows, newRows []pricebook.Row) diff {
	oldByKey := make(map[pricebook.Key]pricebook.Row, len(oldRows))
	for _, r := range oldRows {
		oldByKey[r.Key()] = r
	}

	var d diff
	seen := make(map[pricebook.Key]struct{}, len(newRows))
	for _, r := range newRows {
		seen[r.Key()] = struct{}{}
		prev, ok := oldByKey[r.Key()]
		switch {
		case !ok:
			d.added = append(d.added, r.Key())
		case !prev.SamePrices(r):
			d.changed = append(d.changed, r.Key())
		}
	}
	for _, r := range oldRows {
		if _, ok := seen[r.Key()]; !ok {
			d.removed = append(d.removed, r.Key())
		}
	}

	for _, keys := range [][]pricebook.Key{d.added, d.removed, d.changed} {
		sort.Slice(keys, func(i, j int) bool { return less(keys[i], keys[j]) })
	}
	return d
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

	renderList := func(heading string, keys []pricebook.Key) {
		if len(keys) == 0 {
			return
		}
		write("\n<details>\n<summary>%s</summary>\n\n", heading)
		for _, k := range keys {
			write("- %s\n", k)
		}
		write("</details>\n")
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
