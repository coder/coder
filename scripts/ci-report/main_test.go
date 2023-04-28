package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// To update the golden files:
// make update-golden-files
var updateGoldenFiles = flag.Bool("update", false, "update .golden files")

func TestOutputMatchesGoldenFile(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		// Sample created via:
		//	gotestsum --jsonfile ./scripts/ci-report/testdata/gotests.json.sample -- \
		//	./agent ./cli ./cli/cliui \
		//	-count=1 \
		//	-timeout=5m \
		//	-parallel=24 \
		//	-run='^(TestServer|TestAgent_Session|TestGitAuth$|TestPrompt$)'
		filepath.Join("testdata", "gotests.json.sample"),
		// Sample created via:
		//	gotestsum --jsonfile ./scripts/ci-report/testdata/gotests-timeout.json.sample -- \
		//	./agent -run='^TestAgent_Session' -count=1 -timeout=5m -parallel=24 -timeout=2s
		filepath.Join("testdata", "gotests-timeout.json.sample"),
		// https://github.com/golang/go/issues/57305
		filepath.Join("testdata", "gotests-go-issue-57305.json.sample"),
		filepath.Join("testdata", "gotests-go-issue-57305-parallel.json.sample"),
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			goTests, err := parseGoTestJSON(name)
			if err != nil {
				t.Fatalf("error parsing gotestsum report: %v", err)
			}

			rep, err := parseCIReport(goTests)
			if err != nil {
				t.Fatalf("error parsing ci report: %v", err)
			}

			var b bytes.Buffer
			err = printCIReport(&b, rep)
			if err != nil {
				t.Fatalf("error printing report: %v", err)
			}

			goldenFile := filepath.Join("testdata", "ci-report_"+filepath.Base(name)+".golden")
			got := b.Bytes()
			if updateGoldenFile(t, goldenFile, got) {
				return
			}

			want := readGoldenFile(t, goldenFile)
			if runtime.GOOS == "windows" {
				want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
				got = bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))
			}
			require.Equal(t, string(want), string(got))
		})
	}
}

func readGoldenFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	require.NoError(t, err, "error reading golden file")
	return b
}

func updateGoldenFile(t *testing.T, name string, content []byte) bool {
	t.Helper()
	if *updateGoldenFiles {
		err := os.WriteFile(name, content, 0o600)
		require.NoError(t, err, "error updating golden file")
		return true
	}
	return false
}
