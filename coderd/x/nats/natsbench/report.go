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
// measurement. A Status column appears only for groups that contain an
// invalid run, so clean matrices stay compact.
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

	if withStatus {
		_, _ = b.WriteString("| Replicas | Messages | Pubs/sec | Deliveries/sec | Status |\n")
		_, _ = b.WriteString("|---:|---:|---:|---:|---|\n")
	} else {
		_, _ = b.WriteString("| Replicas | Messages | Pubs/sec | Deliveries/sec |\n")
		_, _ = b.WriteString("|---:|---:|---:|---:|\n")
	}
	for _, row := range rows {
		replicas, messages, pubs, dels, status := rowCells(row)
		_, _ = fmt.Fprintf(b, "| %s | %s | %s | %s |", replicas, messages, pubs, dels)
		if withStatus {
			_, _ = fmt.Fprintf(b, " %s |", status)
		}
		_, _ = b.WriteString("\n")
	}
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

// rowCells returns the replicas, messages, pubs/sec, deliveries/sec,
// and status cells for one result. Invalid runs render INVALID rates
// and a status describing why; valid runs render rates and an "ok"
// status that callers may drop for all-clean groups.
func rowCells(row ScenarioResult) (replicas, messages, pubs, dels, status string) {
	cfg := row.Scenario.Config
	if row.Result != nil {
		cfg = row.Result.Config
	}
	replicas = strconv.Itoa(cfg.Replicas)
	messages = formatInt(int64(cfg.Messages))

	if row.valid() {
		return replicas, messages, formatRate(row.Result.PubsPerSec), formatRate(row.Result.DeliveriesPerSec), "ok"
	}
	return replicas, messages, "INVALID", "INVALID", shortReason(row)
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
