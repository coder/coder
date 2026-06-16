package main

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// ScenarioResult pairs a scenario with its run outcome.
type ScenarioResult struct {
	Scenario Scenario
	Result   *Result
	Err      error
}

// valid reports whether the run produced trustworthy numbers: it
// completed without error and recorded zero drops.
func (r ScenarioResult) valid() bool {
	return r.Err == nil && r.Result != nil && r.Result.Drops == 0
}

// RenderMarkdown writes grouped markdown tables, one per payload size
// in first-seen order. Invalid runs (errors or any drops) render
// INVALID instead of throughput numbers; a run with drops is never a
// measurement.
func RenderMarkdown(w io.Writer, results []ScenarioResult) error {
	var b strings.Builder
	for gi, group := range groupByPayload(results) {
		if gi > 0 {
			_, _ = b.WriteString("\n")
		}
		_, _ = fmt.Fprintf(&b, "### Payload %s\n\n", formatPayload(group.payload))
		_, _ = b.WriteString("| Scenario | Replicas | Messages | Pubs/sec | Deliveries/sec | Drops | Notes |\n")
		_, _ = b.WriteString("|---|---:|---:|---:|---:|---:|---|\n")
		for _, row := range group.rows {
			renderRow(&b, row)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
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

func renderRow(b *strings.Builder, row ScenarioResult) {
	cfg := row.Scenario.Config
	if row.Result != nil {
		cfg = row.Result.Config
	}

	pubs, dels, drops, notes := "INVALID", "INVALID", "?", ""
	if row.valid() {
		pubs = formatRate(row.Result.PubsPerSec)
		dels = formatRate(row.Result.DeliveriesPerSec)
		drops = "0"
	} else {
		if row.Result != nil {
			drops = formatInt(row.Result.Drops)
		}
		notes = shortReason(row)
	}
	_, _ = fmt.Fprintf(b, "| %s | %d | %s | %s | %s | %s | %s |\n",
		row.Scenario.Name, cfg.Replicas, formatInt(int64(cfg.Messages)), pubs, dels, drops, notes)
}

// shortReason summarizes why a run is invalid in one table-safe line.
func shortReason(row ScenarioResult) string {
	if row.Err == nil {
		return "dropped messages"
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
	return formatInt(int64(math.Round(rate)))
}

// formatInt renders an integer with comma thousands separators.
func formatInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			_ = b.WriteByte(',')
		}
		_, _ = b.WriteRune(digit)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// formatPayload renders a payload size as KiB when it divides evenly.
func formatPayload(size int) string {
	if size >= 1024 && size%1024 == 0 {
		return fmt.Sprintf("%d KiB", size/1024)
	}
	return fmt.Sprintf("%d B", size)
}
