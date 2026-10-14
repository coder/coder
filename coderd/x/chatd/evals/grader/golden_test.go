package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// TestGoldenCases checks every golden case offline: its hidden tests must
// fail on the untouched seed and pass once the reference solution is
// applied, with output that reports each hidden test passing. A case that
// passes on the seed grades nothing, and one that fails with its solution
// can never be solved. Workspace code that ends the test process early
// with status 0 must leave a report missing.
func TestGoldenCases(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("golden commands use sh")
	}
	files, err := filepath.Glob(filepath.Join("..", "*", "testdata", "golden", "*", "golden.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		caseDir := filepath.Dir(file)
		suiteDir := filepath.Dir(filepath.Dir(filepath.Dir(caseDir)))
		t.Run(filepath.Base(suiteDir)+"/"+filepath.Base(caseDir), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			var golden struct {
				Seed    string   `json:"seed"`
				Command string   `json:"command"`
				Reports []string `json:"reports"`
			}
			require.NoError(t, json.Unmarshal(data, &golden))
			require.NotEmpty(t, golden.Command)
			tool := strings.Fields(golden.Command)[0]
			if _, err := exec.LookPath(tool); err != nil {
				t.Skipf("%s is not installed", tool)
			}
			seed := filepath.Join(suiteDir, "testdata", "seeds", golden.Seed)
			hidden := filepath.Join(caseDir, "hidden")
			solution := filepath.Join(caseDir, "solution")
			require.DirExists(t, hidden)
			require.DirExists(t, solution)

			out, err := runGolden(t, golden.Command, seed, hidden)
			require.Error(t, err, "hidden tests pass on the unmodified seed:\n%s", out)
			out, err = runGolden(t, golden.Command, seed, solution, hidden)
			require.NoError(t, err, "hidden tests fail with the reference solution:\n%s", out)
			reports := append(slices.Clone(golden.Reports), hiddenPassReports(t, hidden)...)
			require.NotEmpty(t, reports, "nothing shows that the hidden tests ran")
			require.Empty(t, missingReports(reports, out), "the reference solution's output lacks reports:\n%s", out)

			exit, ok := earlyExitLayer(t, golden.Command, []string{solution, seed}, hidden)
			if !ok {
				return
			}
			out, err = runGolden(t, golden.Command, seed, solution, hidden, exit)
			require.NoError(t, err, "the early exit did not end the tests with status 0:\n%s", out)
			require.NotEmpty(t, missingReports(reports, out), "every report is present although the tests exited early:\n%s", out)
		})
	}
}

// missingReports returns the reports that no line of out matches, the
// way the hidden-test command checks them with grep -E.
func missingReports(reports []string, out string) []string {
	var missing []string
	for _, r := range reports {
		if !regexp.MustCompile("(?m)" + r).MatchString(out) {
			missing = append(missing, r)
		}
	}
	return missing
}

// hiddenPassReports returns the lines that report each test in the hidden
// files passing. It applies the rules of passReports in shared.ts, which
// builds the same reports into each case's hidden-test command; keep the
// two in step.
func hiddenPassReports(t *testing.T, hidden string) []string {
	t.Helper()
	var reports []string
	err := filepath.WalkDir(hidden, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(hidden, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		base := path.Base(rel)
		switch {
		case pythonTestFile.MatchString(base):
			module := strings.ReplaceAll(strings.TrimSuffix(rel, ".py"), "/", `\.`)
			testCase := ""
			for _, line := range strings.Split(string(data), "\n") {
				if m := pythonTestCase.FindStringSubmatch(line); m != nil {
					testCase = m[1]
				}
				if m := pythonTestMethod.FindStringSubmatch(line); m != nil && testCase != "" {
					reports = append(reports, `^`+m[1]+` \(`+module+`\.`+testCase+`(\.`+m[1]+`)?\) \.\.\. ok$`)
				}
			}
		case strings.HasSuffix(base, "_test.go"):
			for _, m := range goTestFunc.FindAllStringSubmatch(string(data), -1) {
				reports = append(reports, `^--- PASS: `+m[1]+` \(`)
			}
		case strings.HasSuffix(base, ".test.js"):
			for _, m := range nodeTestCall.FindAllStringSubmatch(string(data), -1) {
				reports = append(reports, `^ok [0-9]+ - `+regexp.QuoteMeta(m[1])+`$`)
			}
		}
		return nil
	})
	require.NoError(t, err)
	return reports
}

var (
	pythonTestFile   = regexp.MustCompile(`^test\w*\.py$`)
	pythonTestCase   = regexp.MustCompile(`^class (\w+)\(`)
	pythonTestMethod = regexp.MustCompile(`^ {4}def (test\w*)\(self\b`)
	goTestFunc       = regexp.MustCompile(`(?m)^func (Test\w+)\(\w+ \*testing\.T\)`)
	goPackage        = regexp.MustCompile(`(?m)^package (\w+)`)
	nodeTestCall     = regexp.MustCompile(`(?m)^test\("([^"\\]+)"`)
	nodeRequire      = regexp.MustCompile(`require\("(\.\.?/[^"]+)"\)`)
)

// earlyExitLayer returns a layer of workspace changes that ends the test
// process early with status 0, the way an agent could: os._exit(0) in
// tests/__init__.py for unittest, a TestMain that never runs the tests
// for go test, and process.exit(0) at the top of each module a hidden
// node test requires, read from the first of sources that has it. It
// reports false for a command it has no early exit for.
func earlyExitLayer(t *testing.T, command string, sources []string, hidden string) (string, bool) {
	t.Helper()
	dir := t.TempDir()
	write := func(rel string, data []byte) {
		target := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(t, os.WriteFile(target, data, 0o600))
	}
	switch {
	case strings.Contains(command, "-m unittest"):
		write("tests/__init__.py", []byte("import os\nos._exit(0)\n"))
	case strings.HasPrefix(command, "go test"):
		err := filepath.WalkDir(hidden, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, "_test.go") {
				return err
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(hidden, filepath.Dir(p))
			if err != nil {
				return err
			}
			pkg := goPackage.FindStringSubmatch(string(data))
			require.NotNil(t, pkg, "%s has no package clause", p)
			write(path.Join(filepath.ToSlash(rel), "zz_exit_test.go"), []byte("package "+pkg[1]+"\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(*testing.M) { os.Exit(0) }\n"))
			return nil
		})
		require.NoError(t, err)
	case strings.HasPrefix(command, "node --test"):
		err := filepath.WalkDir(hidden, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".test.js") {
				return err
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			relDir, err := filepath.Rel(hidden, filepath.Dir(p))
			if err != nil {
				return err
			}
			for _, m := range nodeRequire.FindAllStringSubmatch(string(data), -1) {
				rel := path.Join(filepath.ToSlash(relDir), m[1])
				for _, src := range sources {
					module, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(rel)))
					if err == nil {
						write(rel, append([]byte("process.exit(0);\n"), module...))
						break
					}
					require.ErrorIs(t, err, fs.ErrNotExist)
				}
			}
			return nil
		})
		require.NoError(t, err)
	default:
		return "", false
	}
	return dir, true
}

// runGolden copies the layers into a fresh directory, later layers winning,
// and runs command there the way the hidden_tests grader does.
func runGolden(t *testing.T, command string, layers ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for _, layer := range layers {
		require.NoError(t, copyTree(layer, dir))
	}
	tmp := t.TempDir()
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + tmp,
		"TMPDIR=" + tmp,
		"GOCACHE=" + goEnv(t, "GOCACHE"),
		"GOPATH=" + filepath.Join(tmp, "gopath"),
		"GOFLAGS=-mod=mod",
		"GOPROXY=off",
		"GOTOOLCHAIN=local",
		"PYTHONDONTWRITEBYTECODE=1",
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func goEnv(t *testing.T, key string) string {
	t.Helper()
	out, err := exec.Command("go", "env", key).Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
