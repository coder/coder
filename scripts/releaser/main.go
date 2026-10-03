package main

import (
	"errors"
	"os"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func main() {
	var dryRun bool

	cmd := &serpent.Command{
		Use:   "releaser",
		Short: "Release tooling for coder/coder.",
		Long: "Tag and publish releases for coder/coder.\n\n" +
			"releaser runs the interactive release wizard: it walks the\n" +
			"operator through tagging, pushing, and triggering the release\n" +
			"workflow.",
		Options: serpent.OptionSet{
			{
				Name:        "dry-run",
				Flag:        "dry-run",
				Description: "Print mutating commands instead of executing them.",
				Value:       serpent.BoolOf(&dryRun),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			return Run(inv, dryRun)
		},
	}

	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		if errors.Is(err, cliui.ErrCanceled) {
			os.Exit(1)
		}
		// Unwrap serpent's "running command ..." wrapper to keep output
		// clean.
		var runErr *serpent.RunCommandError
		if errors.As(err, &runErr) {
			err = runErr.Err
		}
		pretty.Fprintf(os.Stderr, cliui.DefaultStyles.Error, "Error: %s\n", err)
		os.Exit(1)
	}
}
