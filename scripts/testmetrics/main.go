// Command testmetrics runs Go tests one at a time and writes per-test resource metrics.
package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

const waitMarker = "CODER_TESTMETRICS eventually_ns="

type config struct {
	Packages string
	Run      string
	Output   string
	Timeout  time.Duration
}

type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

type testCase struct {
	Package string
	Name    string
}

type metrics struct {
	Package             string
	Test                string
	Status              string
	Wall                time.Duration
	UserCPU             time.Duration
	SystemCPU           time.Duration
	MemoryBytes         int64
	ResourceAvailable   bool
	EventuallyWait      time.Duration
	EventuallyCallCount int64
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.Packages, "packages", "./...", "comma-separated package patterns")
	flag.StringVar(&cfg.Run, "run", ".*", "regular expression selecting tests")
	flag.StringVar(&cfg.Output, "output", "testmetrics.csv", "CSV output path, or - for stdout")
	flag.DurationVar(&cfg.Timeout, "timeout", 20*time.Minute, "timeout for each test process")
	flag.Parse()

	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout, stderr io.Writer) error {
	if cfg.Packages == "" {
		return xerrors.New("packages must not be empty")
	}
	if cfg.Timeout <= 0 {
		return xerrors.New("timeout must be positive")
	}

	packages, err := listPackages(cfg.Packages)
	if err != nil {
		return err
	}
	selected, err := listTests(packages, cfg.Run)
	if err != nil {
		return err
	}

	rows := make([]metrics, 0, len(selected))
	for _, test := range selected {
		row, err := runTest(test, cfg.Timeout, stderr)
		if err != nil {
			return err
		}
		rows = append(rows, row...)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Wall != rows[j].Wall {
			return rows[i].Wall > rows[j].Wall
		}
		if rows[i].Package != rows[j].Package {
			return rows[i].Package < rows[j].Package
		}
		return rows[i].Test < rows[j].Test
	})
	return writeCSV(cfg.Output, stdout, rows)
}

func listPackages(patterns string) ([]string, error) {
	args := []string{"list"}
	for _, pattern := range strings.Split(patterns, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" {
			args = append(args, pattern)
		}
	}
	cmd := exec.Command("go", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, xerrors.Errorf("list packages: %w", err)
	}
	var packages []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			packages = append(packages, line)
		}
	}
	return packages, nil
}

func listTests(packages []string, runPattern string) ([]testCase, error) {
	matcher, err := regexp.Compile(runPattern)
	if err != nil {
		return nil, xerrors.Errorf("compile run pattern: %w", err)
	}
	var tests []testCase
	for _, pkg := range packages {
		cmd := exec.Command("go", "test", "-run", "^$", "-list", ".", pkg)
		out, err := cmd.Output()
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				return nil, xerrors.Errorf("list tests in %s: %s", pkg, strings.TrimSpace(string(exitErr.Stderr)))
			}
			return nil, xerrors.Errorf("list tests in %s: %w", pkg, err)
		}
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			name := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(name, "Test") && matcher.MatchString(name) {
				tests = append(tests, testCase{Package: pkg, Name: name})
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, xerrors.Errorf("read tests in %s: %w", pkg, err)
		}
	}
	sort.Slice(tests, func(i, j int) bool {
		if tests[i].Package != tests[j].Package {
			return tests[i].Package < tests[j].Package
		}
		return tests[i].Name < tests[j].Name
	})
	return tests, nil
}

func runTest(test testCase, timeout time.Duration, stderr io.Writer) ([]metrics, error) {
	pattern := "^(?:" + regexp.QuoteMeta(test.Name) + ")$"
	cmd := exec.Command("go", "test", "-json", "-count=1", "-p=1", "-parallel=1", "-run", pattern, test.Package)
	cmd.Env = append(os.Environ(), "CODER_TESTMETRICS=1")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	started := time.Now()
	if err := runCommand(cmd, timeout); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s %s: %v\n", test.Package, test.Name, err)
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, xerrors.Errorf("run %s %s: %w", test.Package, test.Name, err)
		}
	}
	resource := resourceUsage(cmd.ProcessState)
	fallbackWall := time.Since(started)
	return parseEvents(output.Bytes(), test, resource, fallbackWall)
}

var runCommand = func(cmd *exec.Cmd, timeout time.Duration) error {
	if timeout <= 0 {
		return cmd.Run()
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return xerrors.Errorf("test timed out after %s", timeout)
	}
}

func parseEvents(data []byte, test testCase, resource resources, fallbackWall time.Duration) ([]metrics, error) {
	var rows []metrics
	waitByName := make(map[string]struct {
		wait  time.Duration
		calls int64
	})
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var event testEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			parseWaitMarker(scanner.Text(), waitByName)
			continue
		}
		if event.Action == "output" {
			parseWaitMarker(event.Output, waitByName)
			continue
		}
		if event.Test == "" || (event.Action != "pass" && event.Action != "fail" && event.Action != "skip") {
			continue
		}
		status := event.Action
		row := metrics{Package: event.Package, Test: event.Test, Status: status, Wall: time.Duration(event.Elapsed * float64(time.Second))}
		if row.Package == "" {
			row.Package = test.Package
		}
		if row.Wall == 0 {
			row.Wall = fallbackWall
		}
		if wait, ok := waitByName[event.Test]; ok {
			row.EventuallyWait, row.EventuallyCallCount = wait.wait, wait.calls
		}
		if row.Test != test.Name {
			row.ResourceAvailable = false
		}
		row.UserCPU, row.SystemCPU, row.MemoryBytes = resource.UserCPU, resource.SystemCPU, resource.MemoryBytes
		row.ResourceAvailable = row.ResourceAvailable || resource.Available && row.Test == test.Name
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []metrics{{Package: test.Package, Test: test.Name, Status: "error", Wall: fallbackWall}}, nil
	}
	return rows, nil
}

func parseWaitMarker(output string, waits map[string]struct {
	wait  time.Duration
	calls int64
},
) {
	index := strings.Index(output, waitMarker)
	if index < 0 {
		return
	}
	fields := strings.Fields(output[index+len(waitMarker):])
	if len(fields) == 0 {
		return
	}
	ns, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return
	}
	name := ""
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "test=") {
			name = strings.TrimPrefix(field, "test=")
		}
	}
	if name == "" {
		return
	}
	value := waits[name]
	value.wait += time.Duration(ns)
	value.calls++
	waits[name] = value
}

func writeCSV(path string, stdout io.Writer, rows []metrics) error {
	var writer = stdout
	var file *os.File
	if path != "-" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
			return xerrors.Errorf("create report directory: %w", err)
		}
		var err error
		file, err = os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return xerrors.Errorf("open report: %w", err)
		}
		defer file.Close()
		writer = file
	}
	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write([]string{"package", "test", "status", "wall_ms", "user_cpu_ms", "system_cpu_ms", "wall_minus_cpu_ms", "memory_peak_bytes", "eventually_wait_ms", "eventually_calls", "resource_scope"}); err != nil {
		return err
	}
	for _, row := range rows {
		resourceScope := "unavailable"
		if row.ResourceAvailable {
			resourceScope = "test-process"
		}
		cpu := row.UserCPU + row.SystemCPU
		wallMinusCPU := ""
		if row.ResourceAvailable {
			wallMinusCPU = formatMillis(maxDuration(row.Wall - cpu))
		}
		record := []string{row.Package, row.Test, row.Status, formatMillis(row.Wall), formatMillis(row.UserCPU), formatMillis(row.SystemCPU), wallMinusCPU, strconv.FormatInt(row.MemoryBytes, 10), formatMillis(row.EventuallyWait), strconv.FormatInt(row.EventuallyCallCount, 10), resourceScope}
		if err := csvWriter.Write(record); err != nil {
			return err
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

func formatMillis(duration time.Duration) string {
	return strconv.FormatFloat(float64(duration)/float64(time.Millisecond), 'f', 3, 64)
}

func maxDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return 0
	}
	return duration
}
