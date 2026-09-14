package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scripts/docgenenv"
)

func TestSentence(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"capitalizes and terminates", "enables embedded NATS pubsub", "Enables embedded NATS pubsub."},
		{"keeps existing period", "Sends notifications.", "Sends notifications."},
		{"collapses a wrapped comment", "enables the\n MCP HTTP server\n", "Enables the MCP HTTP server."},
		{"empty stays empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sentence(tc.in); got != tc.want {
				t.Errorf("sentence(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestReadDescriptions(t *testing.T) {
	t.Parallel()

	src := `package codersdk

type Experiment string

const (
	ExperimentExample  Experiment = "example"  // This isn't used for anything.
	ExperimentWrapped  Experiment = "wrapped"  // Spans a comment
	// ExperimentDocumented has a doc comment instead of a trailing one.
	ExperimentDocumented Experiment = "documented"
	ExperimentBare     Experiment = "bare"
)

const NotAnExperiment = "ignored"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "deployment.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	got, err := readDescriptions(path)
	if err != nil {
		t.Fatalf("readDescriptions: %v", err)
	}

	want := map[string]string{
		"example":    "This isn't used for anything.",
		"wrapped":    "Spans a comment.",
		"documented": "ExperimentDocumented has a doc comment instead of a trailing one.",
		"bare":       "",
	}
	for key, wantDesc := range want {
		if got[key] != wantDesc {
			t.Errorf("description[%q] = %q, want %q", key, got[key], wantDesc)
		}
	}
	if _, ok := got["ignored"]; ok {
		t.Error("a non-Experiment constant should not be collected")
	}
}

func TestReadDescriptionsRejectsFileWithoutExperiments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "empty.go")
	if err := os.WriteFile(path, []byte("package codersdk\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if _, err := readDescriptions(path); err == nil {
		t.Error("readDescriptions should fail when the file declares no experiments")
	}
}

// TestRenderCoversEveryKnownExperiment guards the page's central promise: it
// lists every experiment a deployment can enable. An experiment added to
// ExperimentsKnown without a description comment fails generation rather than
// publishing a blank cell.
func TestRenderCoversEveryKnownExperiment(t *testing.T) {
	t.Parallel()

	descriptions, err := readDescriptions("../../codersdk/deployment.go")
	if err != nil {
		t.Fatalf("readDescriptions: %v", err)
	}

	page, err := render(docgenenv.Route{Title: "Experiments"}, descriptions)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, exp := range codersdk.ExperimentsKnown {
		if !strings.Contains(page, "`"+string(exp)+"`") {
			t.Errorf("generated page is missing experiment %q", exp)
		}
		if !strings.Contains(page, exp.DisplayName()) {
			t.Errorf("generated page is missing display name for %q", exp)
		}
	}
}

func TestRenderFailsOnMissingDescription(t *testing.T) {
	t.Parallel()

	if _, err := render(docgenenv.Route{Title: "Experiments"}, map[string]string{"unrelated": "x"}); err == nil {
		t.Error("render should fail when a known experiment has no description")
	}
}
