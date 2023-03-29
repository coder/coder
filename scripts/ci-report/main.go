package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
)

func main() {
	if len(os.Args) != 2 {
		_, _ = fmt.Println("usage: ci-report <gotests.json>")
		os.Exit(1)
	}
	name := os.Args[1]

	goTests, err := parseGoTestJSON(name)
	if err != nil {
		_, _ = fmt.Printf("error parsing gotestsum report: %v", err)
		os.Exit(1)
	}

	rep, err := parseCIReport(goTests)
	if err != nil {
		_, _ = fmt.Printf("error parsing ci report: %v", err)
		os.Exit(1)
	}

	err = printCIReport(os.Stdout, rep)
	if err != nil {
		_, _ = fmt.Printf("error printing report: %v", err)
		os.Exit(1)
	}
}

func parseGoTestJSON(name string) (GotestsumReport, error) {
	f, err := os.Open(name)
	if err != nil {
		return GotestsumReport{}, xerrors.Errorf("error opening gotestsum json file: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var report GotestsumReport
	for {
		var e GotestsumReportEntry
		err = dec.Decode(&e)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return GotestsumReport{}, xerrors.Errorf("error decoding json: %w", err)
		}
		e.Package = strings.TrimPrefix(e.Package, "github.com/coder/coder/")
		report = append(report, e)
	}

	return report, nil
}

func parseCIReport(report GotestsumReport) (CIReport, error) {
	packagesSortedByName := []string{}
	packageTimes := map[string]float64{}
	packageFail := map[string]int{}
	packageSkip := map[string]bool{}
	testTimes := map[string]float64{}
	testSkip := map[string]bool{}
	testOutput := map[string]string{}
	testSortedByName := []string{}
	for i, e := range report {
		switch e.Action {
		// A package/test may fail or pass.
		case Fail:
			if e.Test == "" {
				packageTimes[e.Package] = *e.Elapsed
			} else {
				packageFail[e.Package]++
				name := e.Package + "." + e.Test
				testTimes[name] = *e.Elapsed
			}
		case Pass:
			if e.Test == "" {
				packageTimes[e.Package] = *e.Elapsed
			} else {
				name := e.Package + "." + e.Test
				delete(testOutput, name)
				testTimes[name] = *e.Elapsed
			}

		// Gather all output (deleted when irrelevant).
		case Output:
			if e.Test != "" {
				name := e.Package + "." + e.Test
				testOutput[name] += e.Output
			}

		// Packages start, tests run and either may be skipped.
		case Start:
			packagesSortedByName = append(packagesSortedByName, e.Package)
		case Run:
			name := e.Package + "." + e.Test
			testSortedByName = append(testSortedByName, name)
		case Skip:
			if e.Test == "" {
				packageSkip[e.Package] = true
			} else {
				name := e.Package + "." + e.Test
				testSkip[name] = true
				delete(testOutput, name)
			}

		// Ignore.
		case Cont:
		case Pause:

		default:
			return CIReport{}, xerrors.Errorf("unknown action: %v in entry %d (%v)", e.Action, i, e)
		}
	}

	sortAZ := func(a, b string) bool { return a < b }
	slices.SortFunc(packagesSortedByName, sortAZ)
	slices.SortFunc(testSortedByName, sortAZ)

	var rep CIReport

	for _, pkg := range packagesSortedByName {
		rep.Packages = append(rep.Packages, PackageReport{
			Name:      pkg,
			Time:      packageTimes[pkg],
			Skip:      packageSkip[pkg],
			Fail:      packageFail[pkg] > 0,
			NumFailed: packageFail[pkg],
		})
	}

	for _, test := range testSortedByName {
		names := strings.SplitN(test, ".", 2)
		skip := testSkip[test]
		out, fail := testOutput[test]
		rep.Tests = append(rep.Tests, TestReport{
			Package: names[0],
			Name:    names[1],
			Time:    testTimes[test],
			Skip:    skip,
			Fail:    fail,
			Output:  out,
		})
	}

	return rep, nil
}

func printCIReport(dst io.Writer, rep CIReport) error {
	enc := json.NewEncoder(dst)
	enc.SetIndent("", "  ")
	err := enc.Encode(rep)
	if err != nil {
		return xerrors.Errorf("error encoding json: %w", err)
	}
	return nil
}

type CIReport struct {
	Packages []PackageReport `json:"packages"`
	Tests    []TestReport    `json:"tests"`
}

type PackageReport struct {
	Name      string  `json:"name"`
	Time      float64 `json:"time"`
	Skip      bool    `json:"skip,omitempty"`
	Fail      bool    `json:"fail,omitempty"`
	NumFailed int     `json:"num_failed,omitempty"`
}

type TestReport struct {
	Package string  `json:"package"`
	Name    string  `json:"name"`
	Time    float64 `json:"time"`
	Skip    bool    `json:"skip,omitempty"`
	Fail    bool    `json:"fail,omitempty"`
	Output  string  `json:"output,omitempty"`
}

type GotestsumReport []GotestsumReportEntry

type GotestsumReportEntry struct {
	Time    time.Time `json:"Time"`
	Action  Action    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test,omitempty"`
	Output  string    `json:"Output,omitempty"`
	Elapsed *float64  `json:"Elapsed,omitempty"`
}

type Action string

const (
	Cont   Action = "cont"
	Fail   Action = "fail"
	Output Action = "output"
	Pass   Action = "pass"
	Pause  Action = "pause"
	Run    Action = "run"
	Skip   Action = "skip"
	Start  Action = "start"
)
