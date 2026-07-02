package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

const (
	owner = "coder"
	repo  = "coder"
)

func main() {
	var (
		releaseType    string
		ref            string
		commitSHA      string
		versionStr     string
		prevVersionStr string
		notesFile      string
		stable         bool
		dryRun         bool
	)

	dryRunOption := serpent.Option{
		Name:        "dry-run",
		Flag:        "dry-run",
		Description: "Print mutating commands instead of executing them.",
		Value:       serpent.BoolOf(&dryRun),
	}

	// newExecutor returns the appropriate CommandExecutor based on
	// the --dry-run flag.
	newExecutor := func() CommandExecutor {
		if dryRun {
			return newDryRunExecutor(os.Stderr)
		}
		return realExecutor{}
	}

	cmd := &serpent.Command{
		Use:   "release-action <subcommand>",
		Short: "Non-interactive, CI-oriented release tool for coder/coder.",
		Children: []*serpent.Command{
			{
				Use:   "calculate-version",
				Short: "Calculate the next release version from git state.",
				Options: serpent.OptionSet{
					{
						Name:        "type",
						Flag:        "type",
						Description: "Release type: rc, release, or create-release-branch.",
						Value:       serpent.StringOf(&releaseType),
						Required:    true,
					},
					{
						Name:        "ref",
						Flag:        "ref",
						Description: "Git ref (branch name) the workflow is running on.",
						Value:       serpent.StringOf(&ref),
						Required:    true,
					},
					{
						Name:        "commit",
						Flag:        "commit",
						Description: "Commit SHA to tag (defaults to HEAD of --ref if empty).",
						Value:       serpent.StringOf(&commitSHA),
					},
					dryRunOption,
				},
				Handler: func(inv *serpent.Invocation) error {
					result, err := calculateNextVersion(newExecutor(), releaseType, ref, commitSHA)
					if err != nil {
						return err
					}
					_, _ = fmt.Fprintln(inv.Stdout, result.String())
					return nil
				},
			},
			{
				Use:   "prepare-release",
				Short: "Calculate version, create and push tag (and optionally release branch).",
				Options: serpent.OptionSet{
					{
						Name:        "type",
						Flag:        "type",
						Description: "Release type: rc, release, or create-release-branch.",
						Value:       serpent.StringOf(&releaseType),
						Required:    true,
					},
					{
						Name:        "ref",
						Flag:        "ref",
						Description: "Git ref (branch name) the workflow is running on.",
						Value:       serpent.StringOf(&ref),
						Required:    true,
					},
					{
						Name:        "commit",
						Flag:        "commit",
						Description: "Commit SHA to tag (defaults to HEAD of --ref if empty).",
						Value:       serpent.StringOf(&commitSHA),
					},
					dryRunOption,
				},
				Handler: func(inv *serpent.Invocation) error {
					result, err := prepareRelease(newExecutor(), releaseType, ref, commitSHA)
					if err != nil {
						return err
					}
					_, _ = fmt.Fprintln(inv.Stdout, result.String())
					return nil
				},
			},
			{
				Use:   "generate-notes",
				Short: "Generate release notes from commit log and PR metadata.",
				Options: serpent.OptionSet{
					{
						Name:        "version",
						Flag:        "version",
						Description: "New release version (e.g. v2.21.0).",
						Value:       serpent.StringOf(&versionStr),
						Required:    true,
					},
					{
						Name:        "previous-version",
						Flag:        "previous-version",
						Description: "Previous release version (e.g. v2.20.0).",
						Value:       serpent.StringOf(&prevVersionStr),
						Required:    true,
					},
					dryRunOption,
				},
				Handler: func(inv *serpent.Invocation) error {
					newVer, err := parseVersion(versionStr)
					if err != nil {
						return xerrors.Errorf("parse --version: %w", err)
					}
					prevVer, err := parseVersion(prevVersionStr)
					if err != nil {
						return xerrors.Errorf("parse --previous-version: %w", err)
					}
					notes, err := generateReleaseNotes(newExecutor(), newVer, prevVer)
					if err != nil {
						return err
					}
					_, _ = fmt.Fprint(inv.Stdout, notes)
					return nil
				},
			},
			{
				Use:   "publish",
				Short: "Publish a GitHub release with assets and checksums.",
				Options: serpent.OptionSet{
					{
						Name:        "version",
						Flag:        "version",
						Description: "Release version tag (e.g. v2.21.0).",
						Value:       serpent.StringOf(&versionStr),
						Required:    true,
					},
					{
						Name:        "stable",
						Flag:        "stable",
						Description: "Mark this release as the latest stable release.",
						Value:       serpent.BoolOf(&stable),
					},
					{
						Name:        "release-notes-file",
						Flag:        "release-notes-file",
						Description: "Path to release notes markdown file.",
						Value:       serpent.StringOf(&notesFile),
						Required:    true,
					},
					dryRunOption,
				},
				Handler: func(inv *serpent.Invocation) error {
					assets := inv.Args
					if len(assets) == 0 {
						return xerrors.New("no asset files provided as arguments")
					}
					return publishRelease(newExecutor(), versionStr, stable, notesFile, assets)
				},
			},
		},
	}

	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		// Unwrap serpent's "running command ..." wrapper to keep output clean.
		var runErr *serpent.RunCommandError
		if errors.As(err, &runErr) {
			err = runErr.Err
		}
		_, _ = fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}
