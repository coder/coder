package main

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// ScenarioResult pairs a scenario with its run outcome.
type ScenarioResult struct {
	Scenario Scenario
	Result   *Result
	Err      error
}

// valid reports whether the run produced trustworthy numbers: it
// completed without error. Dropped messages do NOT invalidate a run;
// they are reported in the Drops column alongside the throughput the
// run did achieve.
func (r ScenarioResult) valid() bool {
	return r.Err == nil && r.Result != nil
}

// RenderMarkdown writes grouped markdown tables, one per payload size
// in first-seen order. Failed runs (a non-nil error) render INVALID
// instead of throughput numbers; a Status column appears only for
// groups that contain a failed run, so clean matrices stay compact.
// Dropped messages are a normal metric in the Drops column, not a
// failure.
func RenderMarkdown(w io.Writer, results []ScenarioResult) error {
	var b strings.Builder
	for gi, group := range groupByPayload(results) {
		if gi > 0 {
			_, _ = b.WriteString("\n")
		}
		_, _ = fmt.Fprintf(&b, "### Payload %s\n\n", formatPayload(group.payload))
		renderGroup(&b, group.rows)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func renderGroup(b *strings.Builder, rows []ScenarioResult) {
	withStatus := false
	for _, row := range rows {
		if !row.valid() {
			withStatus = true
			break
		}
	}

	// The Status column is left-aligned (free text); the rest are
	// numeric and right-aligned.
	headers := []string{"Replicas", "Subjects", "Publishers", "Subscribers", "Messages", "Converge", "Pubs/sec", "Deliveries/sec", "Drops"}
	aligns := []alignment{alignRight, alignRight, alignRight, alignRight, alignRight, alignRight, alignRight, alignRight, alignRight}
	if withStatus {
		headers = append(headers, "Status")
		aligns = append(aligns, alignLeft)
	}

	table := [][]string{headers}
	for _, row := range rows {
		cells, status := rowCells(row)
		if withStatus {
			cells = append(cells, status)
		}
		table = append(table, cells)
	}
	writeAlignedTable(b, table, aligns)
}

type alignment int

const (
	alignLeft alignment = iota
	alignRight
)

// writeAlignedTable writes a GitHub-flavored markdown table whose raw
// text also lines up in a fixed-width terminal: every cell is padded to
// its column's widest value. table[0] is the header row.
func writeAlignedTable(b *strings.Builder, table [][]string, aligns []alignment) {
	widths := make([]int, len(table[0]))
	for _, rowCells := range table {
		for col, cell := range rowCells {
			widths[col] = max(widths[col], len(cell))
		}
	}

	writeRow := func(cells []string) {
		_, _ = b.WriteString("|")
		for col, cell := range cells {
			_, _ = fmt.Fprintf(b, " %s |", pad(cell, widths[col], aligns[col]))
		}
		_, _ = b.WriteString("\n")
	}

	writeRow(table[0])

	// Separator row: plain dashes sized to each column.
	_, _ = b.WriteString("|")
	for _, width := range widths {
		_, _ = fmt.Fprintf(b, " %s |", strings.Repeat("-", width))
	}
	_, _ = b.WriteString("\n")

	for _, cells := range table[1:] {
		writeRow(cells)
	}
}

// pad widens s to width, on the left for right-aligned columns and on
// the right otherwise.
func pad(s string, width int, align alignment) string {
	gap := width - len(s)
	if gap <= 0 {
		return s
	}
	if align == alignRight {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

type payloadGroup struct {
	payload int
	rows    []ScenarioResult
}

func groupByPayload(results []ScenarioResult) []payloadGroup {
	var groups []payloadGroup
	index := make(map[int]int)
	for _, result := range results {
		size := result.Scenario.Config.PayloadSize
		gi, ok := index[size]
		if !ok {
			gi = len(groups)
			index[size] = gi
			groups = append(groups, payloadGroup{payload: size})
		}
		groups[gi].rows = append(groups[gi].rows, result)
	}
	return groups
}

// rowCells returns the shape and measured cells for one result, plus a
// status string. The cells are: replicas, subjects, publishers,
// subscribers, messages, cluster convergence time, pubs/sec,
// deliveries/sec, drops. Failed runs render INVALID rates and a status
// describing why; valid runs render rates and an "ok" status that
// callers may drop for all-clean groups. A valid run that dropped
// messages still renders its rates, with the loss in the Drops column.
func rowCells(row ScenarioResult) (cells []string, status string) {
	cfg := row.Scenario.Config
	if row.Result != nil {
		cfg = row.Result.Config
	}
	cells = []string{
		strconv.Itoa(cfg.Replicas),
		strconv.Itoa(cfg.Subjects),
		strconv.Itoa(cfg.Publishers),
		strconv.Itoa(cfg.Subscribers),
		humanize.Comma(int64(cfg.Messages)),
		formatConvergence(row),
	}

	if row.valid() {
		return append(cells,
			formatRate(row.Result.PubsPerSec),
			formatRate(row.Result.DeliveriesPerSec),
			formatDrops(row.Result),
		), "ok"
	}
	return append(cells, "INVALID", "INVALID", formatDrops(row.Result)), shortReason(row)
}

// formatDrops renders the drop count and its percentage of expected
// deliveries, or "-" when there is no result to measure.
func formatDrops(res *Result) string {
	if res == nil {
		return "-"
	}
	if res.Drops == 0 {
		return "0"
	}
	pct := 0.0
	if res.Expected > 0 {
		pct = 100 * float64(res.Drops) / float64(res.Expected)
	}
	return fmt.Sprintf("%s (%.2f%%)", humanize.Comma(res.Drops), pct)
}

// shortReason summarizes why a run failed in one table-safe line.
func shortReason(row ScenarioResult) string {
	if row.Err == nil {
		return "no result"
	}
	msg := row.Err.Error()
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	const maxLen = 80
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "..."
	}
	return strings.ReplaceAll(msg, "|", "\\|")
}

// formatRate renders a rate as a comma-separated integer.
func formatRate(rate float64) string {
	return humanize.Comma(int64(math.Round(rate)))
}

// formatConvergence renders the cluster convergence time, or "-" for
// single-node runs and runs without a result, which have no gate.
func formatConvergence(row ScenarioResult) string {
	if row.Result == nil || row.Result.Config.Replicas <= 1 {
		return "-"
	}
	return row.Result.ConvergenceDuration.Round(100 * time.Microsecond).String()
}

// formatPayload renders a payload size as KiB when it divides evenly.
func formatPayload(size int) string {
	if size >= 1024 && size%1024 == 0 {
		return fmt.Sprintf("%d KiB", size/1024)
	}
	return fmt.Sprintf("%d B", size)
}
