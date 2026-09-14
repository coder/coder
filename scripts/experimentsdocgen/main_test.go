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
		{"capitalizes a multi-byte rune", "éclair experiment", "Éclair experiment."},
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
	ExperimentTrailing Experiment = "trailing" // Uses a trailing comment.
	// ExperimentDocumented has a doc comment instead of a trailing one.
	ExperimentDocumented Experiment = "documented"
	ExperimentBare       Experiment = "bare"
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
		"trailing":   "Uses a trailing comment.",
		"documented": "",
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

func TestReadDisplayNames(t *testing.T) {
	t.Parallel()

	displayNames, err := readDisplayNames("../../codersdk/deployment.go")
	if err != nil {
		t.Fatalf("readDisplayNames: %v", err)
	}
	for _, exp := range codersdk.ExperimentsKnown {
		if !displayNames[string(exp)] {
			t.Errorf("experiment %q has no explicit display name", exp)
		}
	}
}

// TestRenderCoversEveryKnownExperiment guards the page's central promise: it
// lists every experiment a deployment can enable. An experiment added to
// ExperimentsKnown without a description or explicit display name fails
// generation rather than publishing incomplete metadata.
func TestRenderCoversEveryKnownExperiment(t *testing.T) {
	t.Parallel()

	descriptions, err := readDescriptions("../../codersdk/deployment.go")
	if err != nil {
		t.Fatalf("readDescriptions: %v", err)
	}
	displayNames, err := readDisplayNames("../../codersdk/deployment.go")
	if err != nil {
		t.Fatalf("readDisplayNames: %v", err)
	}
	page, err := render(docgenenv.Route{Title: "Experiments"}, codersdk.ExperimentsKnown, codersdk.ExperimentsSafe, descriptions, displayNames)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, exp := range codersdk.ExperimentsKnown {
		if !strings.Contains(page, "`"+string(exp)+"`") {
			t.Errorf("generated page is missing experiment %q", exp)
		}
	}
}

func TestRenderFailsOnMissingDescription(t *testing.T) {
	t.Parallel()

	if _, err := render(docgenenv.Route{Title: "Experiments"}, codersdk.ExperimentsKnown, nil, map[string]string{"unrelated": "x"}, nil); err == nil {
		t.Error("render should fail when a known experiment has no description")
	}
}

func TestRenderFailsOnMissingDisplayName(t *testing.T) {
	t.Parallel()

	_, err := render(
		docgenenv.Route{Title: "Experiments"},
		codersdk.Experiments{codersdk.ExperimentNotifications},
		nil,
		map[string]string{"notifications": "Sends notifications."},
		nil,
	)
	if err == nil {
		t.Error("render should fail when a known experiment has no explicit display name")
	}
}

func TestRenderRendersSafeExperiments(t *testing.T) {
	t.Parallel()

	page, err := render(
		docgenenv.Route{Title: "Experiments"},
		codersdk.Experiments{codersdk.ExperimentNotifications},
		codersdk.Experiments{codersdk.ExperimentNotifications},
		map[string]string{"notifications": "Sends notifications."},
		map[string]bool{"notifications": true},
	)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(page, "These experiments are safe to enable with the wildcard:") {
		t.Error("generated page is missing safe-experiments heading")
	}
	if !strings.Contains(page, "- `notifications`") {
		t.Error("generated page is missing safe experiment")
	}
}

func TestRenderEscapesMarkdownTableCells(t *testing.T) {
	t.Parallel()

	page, err := render(
		docgenenv.Route{Title: "Experiments"},
		codersdk.Experiments{codersdk.ExperimentNotifications},
		nil,
		map[string]string{"notifications": "Sends notifications by email | webhook."},
		map[string]bool{"notifications": true},
	)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(page, "Sends notifications by email \\| webhook.") {
		t.Error("generated page did not escape a Markdown table delimiter")
	}
}
