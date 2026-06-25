package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"golang.org/x/xerrors"
)

const defaultFailuresCapBytes = 4 * 1024 * 1024

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[\x20-\x2f]*[\x40-\x7e]`)

type config struct {
	JSONFile         string
	MarkdownOut      string
	FailuresOut      string
	MaxOutputBytes   int
	MaxFailures      int
	FailuresCapBytes int
}

type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

type testKey struct {
	pkg  string
	test string
}

type failure struct {
	Package string
	Test    string
	Elapsed float64
	Output  string
}

type summary struct {
	Failures             []failure
	DurationSeconds      float64
	PackageFailureCount  int
	MalformedLineWarning int
}

type tailBuffer struct {
	maxBytes int
	value    string
}

func main() {
	cfg := config{MarkdownOut: "-", MaxOutputBytes: 8192, FailuresCapBytes: defaultFailuresCapBytes}
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flags.StringVar(&cfg.JSONFile, "jsonfile", cfg.JSONFile, "path to go test JSON output")
	flags.StringVar(&cfg.MarkdownOut, "markdown-out", cfg.MarkdownOut, "path for Markdown output, or - for stdout")
	flags.StringVar(&cfg.FailuresOut, "failures-out", cfg.FailuresOut, "path for failures NDJSON output")
	flags.IntVar(&cfg.MaxOutputBytes, "max-output-bytes", cfg.MaxOutputBytes, "maximum output bytes captured per failure")
	flags.IntVar(&cfg.MaxFailures, "max-failures", cfg.MaxFailures, "maximum failures to render in Markdown, or 0 for all")
	if err := flags.Parse(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := run(context.Background(), cfg, os.Stdout, os.Stderr, os.Getenv); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config, stdout, stderr io.Writer, getenv func(string) string) error {
	if cfg.JSONFile == "" {
		return xerrors.New("--jsonfile is required")
	}
	if cfg.MarkdownOut == "" {
		cfg.MarkdownOut = "-"
	}
	if cfg.MaxOutputBytes < 0 {
		return xerrors.New("--max-output-bytes must be non-negative")
	}
	if cfg.MaxFailures < 0 {
		return xerrors.New("--max-failures must be non-negative")
	}
	if cfg.FailuresCapBytes <= 0 {
		cfg.FailuresCapBytes = defaultFailuresCapBytes
	}

	stat, err := os.Stat(cfg.JSONFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeEmptyOutputs(cfg)
		}
		return xerrors.Errorf("stat json file: %w", err)
	}
	if stat.Size() == 0 {
		return writeEmptyOutputs(cfg)
	}

	file, err := os.Open(cfg.JSONFile)
	if err != nil {
		return xerrors.Errorf("open json file: %w", err)
	}
	defer file.Close()

	result, err := summarize(ctx, file, cfg.MaxOutputBytes, stderr)
	if err != nil {
		return err
	}
	if cfg.FailuresOut != "" {
		if err := writeFailuresNDJSON(cfg.FailuresOut, result.Failures, cfg.FailuresCapBytes); err != nil {
			return err
		}
	}
	if len(result.Failures) == 0 {
		if cfg.MarkdownOut != "-" {
			return os.WriteFile(cfg.MarkdownOut, nil, 0o600)
		}
		return nil
	}
	markdown := renderMarkdown(result, cfg.MaxFailures, cfg.FailuresOut, getenv("GITHUB_JOB"))
	if cfg.MarkdownOut == "-" {
		_, err = io.WriteString(stdout, markdown)
		return err
	}
	return os.WriteFile(cfg.MarkdownOut, []byte(markdown), 0o600)
}

func writeEmptyOutputs(cfg config) error {
	if cfg.FailuresOut != "" {
		if err := os.WriteFile(cfg.FailuresOut, nil, 0o600); err != nil {
			return err
		}
	}
	if cfg.MarkdownOut != "" && cfg.MarkdownOut != "-" {
		return os.WriteFile(cfg.MarkdownOut, nil, 0o600)
	}
	return nil
}

func summarize(ctx context.Context, r io.Reader, maxOutputBytes int, stderr io.Writer) (summary, error) {
	reader := bufio.NewReader(r)
	buffers := map[testKey]*tailBuffer{}
	failures := map[testKey]failure{}
	packageFailures := map[string]struct{}{}
	var durationSeconds float64
	var malformedWarnings int

	for lineNumber := 1; ; lineNumber++ {
		if err := ctx.Err(); err != nil {
			return summary{}, err
		}
		line, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) && line == "" {
			break
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return summary{}, xerrors.Errorf("read json line: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			malformedWarnings++
			writef(stderr, "warning: skipping malformed go test JSON line %d: %v\n", lineNumber, err)
			continue
		}
		if raw == nil {
			malformedWarnings++
			writef(stderr, "warning: skipping non-object go test JSON line %d\n", lineNumber)
			continue
		}
		var event testEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			malformedWarnings++
			writef(stderr, "warning: skipping malformed go test JSON line %d: %v\n", lineNumber, err)
			continue
		}

		key := testKey{pkg: event.Package, test: event.Test}
		switch event.Action {
		case "output":
			bufferFor(buffers, key, maxOutputBytes).Append(stripANSI(event.Output))
		case "pass", "skip":
			delete(buffers, key)
			delete(failures, key)
			if event.Test == "" {
				delete(packageFailures, event.Package)
				if event.Action == "pass" {
					durationSeconds += event.Elapsed
				}
			}
		case "fail":
			if event.Test == "" {
				durationSeconds += event.Elapsed
				if event.Package != "" {
					packageFailures[event.Package] = struct{}{}
				}
			}
			output := bufferFor(buffers, key, maxOutputBytes).String()
			if output == "" && event.Test != "" {
				output = bufferFor(buffers, testKey{pkg: event.Package}, maxOutputBytes).String()
			}
			failures[key] = failure{
				Package: cmpString(event.Package, "unknown"),
				Test:    displayTestName(event.Test),
				Elapsed: event.Elapsed,
				Output:  strings.ToValidUTF8(output, ""),
			}
		}

		if errors.Is(err, io.EOF) {
			break
		}
	}

	failureList := make([]failure, 0, len(failures))
	for _, item := range failures {
		failureList = append(failureList, item)
	}
	sort.Slice(failureList, func(i, j int) bool {
		if failureList[i].Package != failureList[j].Package {
			return failureList[i].Package < failureList[j].Package
		}
		return failureList[i].Test < failureList[j].Test
	})

	return summary{
		Failures:             failureList,
		DurationSeconds:      durationSeconds,
		PackageFailureCount:  len(packageFailures),
		MalformedLineWarning: malformedWarnings,
	}, nil
}

func bufferFor(buffers map[testKey]*tailBuffer, key testKey, maxOutputBytes int) *tailBuffer {
	buffer := buffers[key]
	if buffer == nil {
		buffer = &tailBuffer{maxBytes: maxOutputBytes}
		buffers[key] = buffer
	}
	return buffer
}

func (b *tailBuffer) Append(output string) {
	if b.maxBytes == 0 || output == "" {
		return
	}
	b.value += output
	if len(b.value) > b.maxBytes {
		b.value = b.value[len(b.value)-b.maxBytes:]
	}
}

func (b *tailBuffer) String() string {
	return strings.ToValidUTF8(b.value, "")
}

func renderMarkdown(result summary, maxFailures int, failuresOut string, githubJob string) string {
	failures := result.Failures
	visibleFailures := failures
	if maxFailures > 0 && len(failures) > maxFailures {
		visibleFailures = failures[:maxFailures]
	}
	packageNames := map[string]struct{}{}
	for _, item := range failures {
		packageNames[item.Package] = struct{}{}
	}

	var builder strings.Builder
	writeBuilderf(&builder, "## Go test failures (%d in %d packages)\n\n", len(failures), len(packageNames))
	writeBuilderf(&builder, "Duration: %s · Packages with failures: %d", formatSeconds(result.DurationSeconds), result.PackageFailureCount)
	if githubJob != "" {
		writeBuilderf(&builder, " · Job: %s", escapeMarkdownLine(githubJob))
	}
	writeBuilderString(&builder, "\n\n")
	writeBuilderString(&builder, "| Package | Test | Elapsed |\n")
	writeBuilderString(&builder, "|---|---|---|\n")
	for _, item := range visibleFailures {
		writeBuilderf(&builder, "| %s | %s | %s |\n",
			escapeTableCell(item.Package),
			escapeTableCell(item.Test),
			formatSeconds(item.Elapsed),
		)
	}
	writeBuilderString(&builder, "\n")

	for _, item := range visibleFailures {
		output := item.Output
		if output == "" {
			output = "No output recorded."
		}
		output = strings.ReplaceAll(strings.ToValidUTF8(output, ""), "```", "``")
		writeBuilderf(&builder, "<details>\n<summary><code>%s</code> :: <code>%s</code> (%s)</summary>\n\n",
			html.EscapeString(item.Package),
			html.EscapeString(item.Test),
			formatSeconds(item.Elapsed),
		)
		writeBuilderString(&builder, "```text\n")
		writeBuilderString(&builder, output)
		if !strings.HasSuffix(output, "\n") {
			writeBuilderString(&builder, "\n")
		}
		writeBuilderString(&builder, "```\n\n</details>\n\n")
	}

	if omitted := len(failures) - len(visibleFailures); omitted > 0 {
		writeBuilderf(&builder, "_... and %d more failed tests omitted.", omitted)
		if failuresOut != "" {
			writeBuilderString(&builder, " Download the failures-only artifact for the full list.")
		}
		writeBuilderString(&builder, "_\n")
	}
	return builder.String()
}

func writeBuilderf(builder *strings.Builder, format string, args ...any) {
	_, _ = fmt.Fprintf(builder, format, args...)
}

func writef(writer io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(writer, format, args...)
}

func writeBuilderString(builder *strings.Builder, value string) {
	_, _ = builder.WriteString(value)
}

func writeFailuresNDJSON(path string, failures []failure, capBytes int) error {
	var output bytes.Buffer
	for index, item := range failures {
		recordLine, err := marshalRecord(failureRecord{
			Package:  item.Package,
			Test:     item.Test,
			ElapsedS: item.Elapsed,
			Output:   strings.ToValidUTF8(item.Output, ""),
		})
		if err != nil {
			return err
		}
		if output.Len()+len(recordLine) <= capBytes {
			_, _ = output.Write(recordLine)
			continue
		}

		remainingAfterCurrent := len(failures) - index - 1
		summaryLine, err := marshalRecord(truncationRecord{Truncated: true, RemainingFailures: remainingAfterCurrent})
		if err != nil {
			return err
		}
		availableForRecord := capBytes - output.Len()
		if remainingAfterCurrent > 0 {
			availableForRecord -= len(summaryLine)
		}
		truncatedLine, ok, err := truncateFailureRecord(item, availableForRecord)
		if err != nil {
			return err
		}
		if ok {
			_, _ = output.Write(truncatedLine)
			if remainingAfterCurrent > 0 && output.Len()+len(summaryLine) <= capBytes {
				_, _ = output.Write(summaryLine)
			}
			break
		}

		summaryLine, err = marshalRecord(truncationRecord{Truncated: true, RemainingFailures: len(failures) - index})
		if err != nil {
			return err
		}
		if output.Len()+len(summaryLine) <= capBytes {
			_, _ = output.Write(summaryLine)
		}
		break
	}
	return os.WriteFile(path, output.Bytes(), 0o600)
}

type failureRecord struct {
	Package         string  `json:"package"`
	Test            string  `json:"test"`
	ElapsedS        float64 `json:"elapsed_s"`
	Output          string  `json:"output"`
	OutputTruncated bool    `json:"output_truncated,omitempty"`
}

type truncationRecord struct {
	Truncated         bool `json:"truncated"`
	RemainingFailures int  `json:"remaining_failures"`
}

func truncateFailureRecord(item failure, capBytes int) ([]byte, bool, error) {
	if capBytes <= 0 {
		return nil, false, nil
	}
	output := []byte(item.Output)
	low, high := 0, len(output)
	var best []byte
	for low <= high {
		mid := low + (high-low)/2
		recordLine, err := marshalRecord(failureRecord{
			Package:         item.Package,
			Test:            item.Test,
			ElapsedS:        item.Elapsed,
			Output:          strings.ToValidUTF8(string(output[:mid]), ""),
			OutputTruncated: true,
		})
		if err != nil {
			return nil, false, err
		}
		if len(recordLine) <= capBytes {
			best = slices.Clone(recordLine)
			low = mid + 1
			continue
		}
		high = mid - 1
	}
	if best == nil {
		return nil, false, nil
	}
	return best, true, nil
}

func marshalRecord(record any) ([]byte, error) {
	line, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	line = append(line, '\n')
	return line, nil
}

func stripANSI(output string) string {
	return ansiEscapePattern.ReplaceAllString(output, "")
}

func displayTestName(name string) string {
	if name == "" {
		return "(package)"
	}
	return name
}

func formatSeconds(seconds float64) string {
	return fmt.Sprintf("%.2fs", seconds)
}

func escapeTableCell(value string) string {
	value = strings.ReplaceAll(value, "|", `\|`)
	value = strings.NewReplacer("\r", " ", "\n", " ", "`", "&#96;").Replace(value)
	return html.EscapeString(value)
}

func escapeMarkdownLine(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}

func cmpString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
