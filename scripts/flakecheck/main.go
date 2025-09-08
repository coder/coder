package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Configurable flags
var (
	flagBase     = flag.String("base", "origin/main", "git ref to diff against (merge-base with HEAD)")
	flagRepeat   = flag.Int("repeat", 10, "number of runs per test selector")
	flagP        = flag.Int("p", 8, "-p package test concurrency")
	flagParallel = flag.Int("parallel", 8, "-parallel test concurrency for t.Parallel")
	flagTimeout  = flag.Duration("timeout", 5*time.Minute, "per-run go test timeout")
	flagWork     = flag.Int("concurrency", 4, "number of selectors to run concurrently")
)

type exitErr struct {
	code int
	msg  string
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			if ee, ok := r.(exitErr); ok {
				_, _ = fmt.Fprintln(os.Stderr, ee.msg)
				os.Exit(ee.code)
			}
			panic(r)
		}
	}()

	flag.Parse()

	changedTests, err := changedTestFiles(*flagBase)
	if err != nil {
		fatalf("detecting changed files: %v", err)
	}
	codePkgs, err := changedCodePackages(*flagBase)
	if err != nil {
		fatalf("detecting changed packages: %v", err)
	}

	packages, err := groupByPackage(changedTests)
	if err != nil {
		fatalf("resolving packages: %v", err)
	}
	// Add all *_test.go files from packages that changed due to non-test code changes
	for pkg := range codePkgs {
		tfs, err := testFilesForPackage(pkg)
		if err != nil {
			fatalf("enumerating tests for %s: %v", pkg, err)
		}
		if len(tfs) == 0 {
			continue
		}
		packages[pkg] = mergeUnique(packages[pkg], tfs)
	}

	if len(packages) == 0 {
		_, _ = fmt.Println("No modified tests or changed packages with tests detected; nothing to run.")
		return
	}

	selectors, err := enumerateSelectors(packages)
	if err != nil {
		fatalf("enumerating selectors: %v", err)
	}
	if len(selectors) == 0 {
		_, _ = fmt.Println("No test selectors found to run.")
		return
	}

	results := runAll(selectors, *flagRepeat, *flagP, *flagParallel, *flagTimeout, *flagWork)

	// Prepare summary output
	var flakyOrBroken []summaryRow
	for _, r := range results {
		if r.Failures > 0 {
			state := "flaky"
			if r.Failures == r.TotalRuns {
				state = "broken"
			}
			flakyOrBroken = append(flakyOrBroken, summaryRow{Pkg: r.Pkg, Selector: r.Selector, Fails: r.Failures, Total: r.TotalRuns, State: state})
		}
	}

	sort.Slice(flakyOrBroken, func(i, j int) bool {
		if flakyOrBroken[i].Pkg == flakyOrBroken[j].Pkg {
			return flakyOrBroken[i].Selector < flakyOrBroken[j].Selector
		}
		return flakyOrBroken[i].Pkg < flakyOrBroken[j].Pkg
	})

	var out bytes.Buffer
	if len(flakyOrBroken) == 0 {
		_, _ = fmt.Fprintf(&out, "No flakes or failures detected across %d selectors with %dx runs.\n", len(results), *flagRepeat)
	} else {
		_, _ = fmt.Fprintf(&out, "Detected flakes or failures (failures/total):\n\n")
		for _, row := range flakyOrBroken {
			_, _ = fmt.Fprintf(&out, "- %s %s: %d/%d (%s)\n", row.Pkg, row.Selector, row.Fails, row.Total, row.State)
		}
	}
	_, _ = fmt.Print(out.String())

	// Exit non-zero if any failures (policy: fail on any flake)
	for _, r := range results {
		if r.Failures > 0 {
			panic(exitErr{code: 1, msg: ""})
		}
	}
}

func fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	panic(exitErr{code: 2, msg: msg})
}

// changedTestFiles returns a list of modified *_test.go files between merge-base(base, HEAD) and HEAD.
func changedTestFiles(base string) ([]string, error) {
	mb, err := runOut("git", "merge-base", base, "HEAD")
	if err != nil {
		// Fallback to base if merge-base fails (shallow clones). Caller should ensure fetch-depth: 0 in CI.
		mb = strings.TrimSpace(base)
	} else {
		mb = strings.TrimSpace(mb)
	}
	out, err := runOut("git", "diff", "--name-only", mb+"..HEAD")
	if err != nil {
		return nil, err
	}
	var tests []string
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		p := scanner.Text()
		if strings.HasSuffix(p, "_test.go") {
			tests = append(tests, p)
		}
	}
	return tests, scanner.Err()
}

// changedCodePackages returns the set of import paths whose non-test *.go files changed.
func changedCodePackages(base string) (map[string]struct{}, error) {
	mb, err := runOut("git", "merge-base", base, "HEAD")
	if err != nil {
		mb = strings.TrimSpace(base)
	} else {
		mb = strings.TrimSpace(mb)
	}
	out, err := runOut("git", "diff", "--name-only", mb+"..HEAD")
	if err != nil {
		return nil, err
	}
	byDir := map[string]struct{}{}
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		p := s.Text()
		if strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			byDir[filepath.Dir(p)] = struct{}{}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	pkgs := map[string]struct{}{}
	for d := range byDir {
		pkg, err := runOut("go", "list", "-f", "{{.ImportPath}}", d)
		if err != nil {
			return nil, fmt.Errorf("go list %s: %w", d, err)
		}
		pkgs[strings.TrimSpace(pkg)] = struct{}{}
	}
	return pkgs, nil
}

// testFilesForPackage lists all *_test.go files for the given import path.
func testFilesForPackage(pkg string) ([]string, error) {
	dir, err := runOut("go", "list", "-f", "{{.Dir}}", pkg)
	if err != nil {
		return nil, fmt.Errorf("go list dir %s: %w", pkg, err)
	}
	dir = strings.TrimSpace(dir)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, "_test.go") {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out, nil
}

// groupByPackage groups files by their import path using `go list` for each directory.
func groupByPackage(files []string) (map[string][]string, error) {
	byDir := map[string][]string{}
	for _, f := range files {
		d := filepath.Dir(f)
		byDir[d] = append(byDir[d], f)
	}
	res := map[string][]string{}
	for d, fs := range byDir {
		pkg, err := runOut("go", "list", "-f", "{{.ImportPath}}", d)
		if err != nil {
			return nil, fmt.Errorf("go list %s: %w", d, err)
		}
		res[strings.TrimSpace(pkg)] = append(res[strings.TrimSpace(pkg)], fs...)
	}
	return res, nil
}

func mergeUnique(a []string, b []string) []string {
	m := map[string]struct{}{}
	for _, x := range a {
		m[x] = struct{}{}
	}
	for _, x := range b {
		m[x] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for x := range m {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

// selector represents a package test selector and its regex for -run.
// Selector is the human-readable joined form, e.g., TestFoo/Sub1/Sub2.
// RunExpr is the regexp string for -run: e.g., ^TestFoo$/^Sub1$/^Sub2$.

type selector struct {
	Pkg      string
	Selector string
	RunExpr  string
}

// enumerateSelectors parses test files and produces the set of granular selectors.
func enumerateSelectors(pkgs map[string][]string) ([]selector, error) {
	var sels []selector
	for pkg, files := range pkgs {
		// Collect per top-level test function its subtests
		tests := map[string][][]string{} // map[TestName]list of subtest paths
		fileset := token.NewFileSet()
		for _, f := range files {
			af, err := parser.ParseFile(fileset, f, nil, parser.ParseComments)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", f, err)
			}
			for _, decl := range af.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Recv != nil || fd.Name == nil {
					continue
				}
				name := fd.Name.Name
				if !strings.HasPrefix(name, "Test") || fd.Type == nil || len(fd.Type.Params.List) == 0 {
					continue
				}
				// Crude check for func(t *testing.T)
				if !isTestingTParam(fd.Type.Params.List[0]) {
					continue
				}
				var paths [][]string
				if fd.Body != nil {
					paths = collectSubtestPaths(fd.Body)
				}
				tests[name] = append(tests[name], paths...)
			}
		}
		// Build selectors: if subtests exist for a top-level test, prefer only subtests; otherwise include the top-level test.
		for testName, subs := range tests {
			if len(subs) == 0 {
				// No subtests found, include top-level
				sels = append(sels, selector{
					Pkg:      pkg,
					Selector: testName,
					RunExpr:  toRunExpr([]string{testName}),
				})
				continue
			}
			// Deduplicate sub-paths
			uniq := map[string]struct{}{}
			for _, path := range subs {
				joined := testName + "/" + strings.Join(path, "/")
				if _, ok := uniq[joined]; ok {
					continue
				}
				uniq[joined] = struct{}{}
				sels = append(sels, selector{
					Pkg:      pkg,
					Selector: joined,
					RunExpr:  toRunExpr(append([]string{testName}, path...)),
				})
			}
		}
	}
	// Sort selectors for determinism
	sort.Slice(sels, func(i, j int) bool {
		if sels[i].Pkg == sels[j].Pkg {
			return sels[i].Selector < sels[j].Selector
		}
		return sels[i].Pkg < sels[j].Pkg
	})
	return sels, nil
}

func isTestingTParam(f *ast.Field) bool {
	// Match *testing.T (without import path resolution).
	// Accept forms: *testing.T or *T if identifier names match in common style.
	star, ok := f.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if ok {
		id, _ := sel.X.(*ast.Ident)
		return id != nil && id.Name == "testing" && sel.Sel != nil && sel.Sel.Name == "T"
	}
	id, ok := star.X.(*ast.Ident)
	return ok && id.Name == "T"
}

// collectSubtestPaths returns all nested subtest name paths discovered under a block.
func collectSubtestPaths(b *ast.BlockStmt) [][]string {
	var out [][]string
	// DFS over statements; whenever we see t.Run("name", func(t *testing.T){...}), capture and recurse
	ast.Inspect(b, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// callee must be something.Run
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || sel.Sel.Name != "Run" {
			return true
		}
		if len(call.Args) != 2 {
			return true
		}
		nameLit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || nameLit.Kind != token.STRING {
			return true
		}
		name := strings.Trim(nameLit.Value, "\"")
		fn, ok := call.Args[1].(*ast.FuncLit)
		if !ok || fn.Type == nil || len(fn.Type.Params.List) == 0 || !isTestingTParam(fn.Type.Params.List[0]) {
			return true
		}
		// Recurse to collect nested subtests inside this function
		var nested [][]string
		if fn.Body != nil {
			nested = collectSubtestPaths(fn.Body)
		}
		if len(nested) == 0 {
			out = append(out, []string{name})
			return true
		}
		for _, npath := range nested {
			out = append(out, append([]string{name}, npath...))
		}
		return true
	})
	return out
}

func toRunExpr(parts []string) string {
	// Build '^A$/^B$/^C$' pattern so each level is anchored
	anchored := make([]string, 0, len(parts))
	for _, p := range parts {
		anchored = append(anchored, "^"+regexp.QuoteMeta(p)+"$")
	}
	return strings.Join(anchored, "/")
}

// runAll executes all selectors with workers and aggregates results
func runAll(selectors []selector, repeat int, p, parallel int, timeout time.Duration, workers int) []testResult {
	type job struct {
		Sel selector
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	mu := sync.Mutex{}
	var results []testResult

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			res := runSelector(j.Sel, repeat, p, parallel, timeout)
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}
	}
	if workers <= 0 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}
	for _, s := range selectors {
		jobs <- job{Sel: s}
	}
	close(jobs)
	wg.Wait()
	return results
}

type testResult struct {
	Pkg       string
	Selector  string
	TotalRuns int
	Passes    int
	Failures  int
}

func runSelector(sel selector, repeat, p, parallel int, timeout time.Duration) testResult {
	res := testResult{Pkg: sel.Pkg, Selector: sel.Selector, TotalRuns: repeat}
	for i := 0; i < repeat; i++ {
		ok, err := runOnce(sel, p, parallel, timeout)
		if err != nil {
			// Treat infrastructure errors as failures to be conservative
			res.Failures++
			continue
		}
		if ok {
			res.Passes++
		} else {
			res.Failures++
		}
	}
	return res
}

// runOnce runs a single go test invocation for the selector and returns true if it passed.
func runOnce(sel selector, p, parallel int, timeout time.Duration) (bool, error) {
	args := []string{"test", "-json", "-count=1", "-run", sel.RunExpr, "-p", fmt.Sprint(p), "-parallel", fmt.Sprint(parallel), fmt.Sprintf("-timeout=%s", timeout.String()), sel.Pkg}
	cmd := exec.Command("go", args...)
	cmd.Env = os.Environ()
	out, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return false, err
	}
	dec := json.NewDecoder(out)
	var passed bool
	var seen bool
	for {
		var ev testEvent
		if err := dec.Decode(&ev); err != nil {
			// If we hit EOF or the pipe closed, rely on the process exit code
			break
		}
		if ev.Test == sel.Selector {
			switch ev.Action {
			case "pass":
				passed = true
				seen = true
			case "fail":
				passed = false
				seen = true
			case "skip":
				passed = false
				seen = true
			}
		}
	}
	waitErr := cmd.Wait()
	// If we didn't see explicit pass/fail for the selector, fall back to process exit code.
	if !seen {
		return waitErr == nil, nil
	}
	return passed, nil
}

type testEvent struct {
	Time    string  `json:"Time"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

type summaryRow struct {
	Pkg      string
	Selector string
	Fails    int
	Total    int
	State    string // flaky or broken
}

func runOut(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	b, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(b), nil
}
