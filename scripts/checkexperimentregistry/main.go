// Command checkexperimentregistry checks experiments/registry.yaml against
// the experiment flags in codersdk.
//
// Every flag in codersdk.ExperimentsKnown must belong to exactly one
// registry entry, and every flag in the registry must be known. Adding,
// renaming, or removing a flag therefore needs a registry change, which
// CODEOWNERS routes to the registry owners.
//
// Usage:
//
//	checkexperimentregistry [registry.yaml]
//
// With no argument it reads experiments/registry.yaml.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/codersdk"
)

const defaultRegistryPath = "experiments/registry.yaml"

var validStages = []string{"early-access", "beta", "ga"}

type registry struct {
	Experiments []experiment `yaml:"experiments"`
	Tools       []tool       `yaml:"tools"`
}

type experiment struct {
	Key    string   `yaml:"key"`
	Stage  string   `yaml:"stage"`
	Since  string   `yaml:"since"`
	Flag   string   `yaml:"flag"`
	Paths  []string `yaml:"paths"`
	Routes []string `yaml:"routes"`
	Docs   []string `yaml:"docs"`
	Notes  string   `yaml:"notes"`
}

type tool struct {
	Paths  []string `yaml:"paths"`
	Routes []string `yaml:"routes"`
	Notes  string   `yaml:"notes"`
}

func main() {
	path := defaultRegistryPath
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	os.Exit(run(path, os.Stderr))
}

// run checks the registry at path, writes problems to stderr, and returns 0
// for success, 1 for problems, or 2 when the registry cannot be read.
func run(path string, stderr io.Writer) int {
	src, err := os.ReadFile(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "checkexperimentregistry: %v\n", err)
		return 2
	}
	problems, err := check(src, codersdk.ExperimentsKnown)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "checkexperimentregistry: parse %s: %v\n", path, err)
		return 2
	}
	for _, problem := range problems {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", path, problem)
	}
	if len(problems) > 0 {
		_, _ = fmt.Fprintf(stderr, "\ncheckexperimentregistry: %d problem(s). Update %s to match codersdk.ExperimentsKnown; CODEOWNERS requires a registry owner to approve the change.\n", len(problems), path)
		return 1
	}
	return 0
}

// check returns the problems in the registry src, compared with the known
// experiment flags.
func check(src []byte, known codersdk.Experiments) ([]string, error) {
	var reg registry
	dec := yaml.NewDecoder(bytes.NewReader(src))
	dec.KnownFields(true)
	if err := dec.Decode(&reg); err != nil {
		return nil, xerrors.Errorf("decode registry: %w", err)
	}

	var problems []string
	keys := make(map[string]bool, len(reg.Experiments))
	flagOwners := make(map[string]string, len(reg.Experiments))
	for i, exp := range reg.Experiments {
		name := exp.Key
		if name == "" {
			name = fmt.Sprintf("experiments[%d]", i)
			problems = append(problems, fmt.Sprintf("%s: missing key", name))
		} else if keys[name] {
			problems = append(problems, fmt.Sprintf("%s: duplicate key", name))
		}
		keys[name] = true

		if !slices.Contains(validStages, exp.Stage) {
			problems = append(problems, fmt.Sprintf("%s: stage %q is not one of %v", name, exp.Stage, validStages))
		}
		if _, err := time.Parse(time.DateOnly, exp.Since); err != nil {
			problems = append(problems, fmt.Sprintf("%s: since %q is not a YYYY-MM-DD date", name, exp.Since))
		}

		if exp.Flag == "" {
			continue
		}
		if owner, ok := flagOwners[exp.Flag]; ok {
			problems = append(problems, fmt.Sprintf("%s: flag %q already belongs to %s", name, exp.Flag, owner))
			continue
		}
		flagOwners[exp.Flag] = name
		if !slices.Contains(known, codersdk.Experiment(exp.Flag)) {
			problems = append(problems, fmt.Sprintf("%s: flag %q is not in codersdk.ExperimentsKnown", name, exp.Flag))
		}
	}

	for _, flag := range known {
		if _, ok := flagOwners[string(flag)]; !ok {
			problems = append(problems, fmt.Sprintf("flag %q from codersdk.ExperimentsKnown has no registry entry", flag))
		}
	}
	return problems, nil
}
