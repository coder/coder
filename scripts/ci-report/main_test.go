package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// To update the golden files:
// make update-golden-files
var updateGoldenFiles = flag.Bool("update", false, "update .golden files")

func TestOutputMatchesGoldenFile(t *testing.T) {
	t.Parallel()

	goTests, err := parseGoTestJSON(filepath.Join("testdata", "gotests-sample.json"))
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

	goldenFile := filepath.Join("testdata", "ci-report.golden")
	got := b.Bytes()
	if updateGoldenFile(t, goldenFile, got) {
		return
	}

	want := readGoldenFile(t, goldenFile)
	require.Equal(t, string(want), string(got))
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
