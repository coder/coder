package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestParseExperimentDescriptions(t *testing.T) {
	t.Parallel()

	src := []byte(`package codersdk

type Experiment string

const (
	ExperimentAlpha Experiment = "alpha" // Enables alpha.
	// Beta has a doc comment instead of a trailing one.
	ExperimentBeta  Experiment = "beta"
	ExperimentGamma Experiment = "gamma"
	NotAnExperiment string     = "ignored" // Different type.
)

const Unrelated = 3
`)

	descriptions, err := parseExperimentDescriptions(src)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"alpha": "Enables alpha.",
		"beta":  "Beta has a doc comment instead of a trailing one.",
		"gamma": "",
	}, descriptions)
}

func TestParseExperimentDescriptionsRejectsInvalidGo(t *testing.T) {
	t.Parallel()

	_, err := parseExperimentDescriptions([]byte("package codersdk\nconst ("))
	require.Error(t, err)
}

func TestBuildExperimentsDoc(t *testing.T) {
	t.Parallel()

	known := codersdk.Experiments{"zeta", "alpha", "mid"}
	safe := codersdk.Experiments{"mid"}
	descriptions := map[string]string{"alpha": "First.", "zeta": "Last."}
	displayName := func(e codersdk.Experiment) string { return "Name of " + string(e) }

	doc := buildExperimentsDoc(known, safe, descriptions, displayName)

	require.Equal(t, 1, doc.SchemaVersion)
	require.Equal(t, []experimentEntry{
		{ID: "alpha", DisplayName: "Name of alpha", Description: "First.", Safe: false},
		{ID: "mid", DisplayName: "Name of mid", Description: "", Safe: true},
		{ID: "zeta", DisplayName: "Name of zeta", Description: "Last.", Safe: false},
	}, doc.Experiments)
}

// The real constants file must parse and every known experiment must have a
// display name, so a new experiment cannot land without documentation metadata.
func TestRealDeploymentFile(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("../../codersdk/deployment.go")
	require.NoError(t, err)
	descriptions, err := parseExperimentDescriptions(src)
	require.NoError(t, err)

	known := documented(codersdk.ExperimentsKnown)
	require.NotContains(t, known, codersdk.ExperimentExample, "the example experiment is never documented")
	require.Len(t, known, len(codersdk.ExperimentsKnown)-1)
	doc := buildExperimentsDoc(known, codersdk.ExperimentsSafe, descriptions, codersdk.Experiment.DisplayName)
	require.Len(t, doc.Experiments, len(known))
	for _, entry := range doc.Experiments {
		require.NotEmpty(t, entry.DisplayName, "experiment %q has no display name", entry.ID)
		require.Contains(t, descriptions, entry.ID, "experiment %q is not declared as a constant", entry.ID)
	}
}
