package main

import (
	"errors"
	"os"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func main() {
	var opts releaseOptions

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
				Value:       serpent.BoolOf(&opts.DryRun),
			},
			{
				Name: "non-interactive",
				Flag: "non-interactive",
				Description: "Answer every prompt with its default, as if pressing Enter. " +
					"Safety checks that default to no fail the release instead.",
				Value: serpent.BoolOf(&opts.NonInteractive),
			},
			{
				Name:        "allow-open-prs",
				Flag:        "allow-open-prs",
				Description: "Continue when open PRs target the release branch.",
				Value:       serpent.BoolOf(&opts.AllowOpenPRs),
			},
			{
				Name:        "allow-breaking-changes",
				Flag:        "allow-breaking-changes",
				Description: "Continue when a patch release contains breaking changes.",
				Value:       serpent.BoolOf(&opts.AllowBreakingChanges),
			},
			{
				Name:        "requested-by",
				Flag:        "requested-by",
				Description: "GitHub login of the user the release runs on behalf of. Recorded in the tag message.",
				Value:       serpent.StringOf(&opts.RequestedBy),
			},
			{
				Name:        "requested-by-run-url",
				Flag:        "requested-by-run-url",
				Description: "URL of the workflow run that requested the release.",
				Value:       serpent.StringOf(&opts.RequestedByRunURL),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			return Run(inv, opts)
		},
	}

	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		// Unwrap serpent's "running command ..." wrapper to keep output
		// clean.
		var runErr *serpent.RunCommandError
		if errors.As(err, &runErr) {
			err = runErr.Err
		}
		// An interactive cancel is the operator's own choice, so there
		// is nothing to report. In non-interactive mode the error says
		// which check stopped the release.
		if errors.Is(err, cliui.ErrCanceled) && !opts.NonInteractive {
			os.Exit(1)
		}
		pretty.Fprintf(os.Stderr, cliui.DefaultStyles.Error, "Error: %s\n", err)
		os.Exit(1)
	}
}
