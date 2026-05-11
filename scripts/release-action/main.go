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
		releaseType     string
		commitSHA       string
		branch          string
		versionStr      string
		prevVersionStr  string
		channel         string
	)

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
						Name:        "commit",
						Flag:        "commit",
						Description: "Commit SHA to use (for rc and create-release-branch).",
						Value:       serpent.StringOf(&commitSHA),
					},
					{
						Name:        "branch",
						Flag:        "branch",
						Description: "Branch name (for rc from branch, or release).",
						Value:       serpent.StringOf(&branch),
					},
				},
				Handler: func(inv *serpent.Invocation) error {
					result, err := calculateNextVersion(releaseType, commitSHA, branch)
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
					{
						Name:        "channel",
						Flag:        "channel",
						Description: "Release channel (stable or rc).",
						Value:       serpent.StringOf(&channel),
						Required:    true,
					},
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
					notes, err := generateReleaseNotes(newVer, prevVer, channel)
					if err != nil {
						return err
					}
					_, _ = fmt.Fprint(inv.Stdout, notes)
					return nil
				},
			},
			{
				Use:   "update-docs",
				Short: "Update release calendar and version pragmas in docs.",
				Options: serpent.OptionSet{
					{
						Name:        "version",
						Flag:        "version",
						Description: "Release version (e.g. v2.21.0).",
						Value:       serpent.StringOf(&versionStr),
						Required:    true,
					},
					{
						Name:        "channel",
						Flag:        "channel",
						Description: "Release channel (stable or rc).",
						Value:       serpent.StringOf(&channel),
						Required:    true,
					},
				},
				Handler: func(inv *serpent.Invocation) error {
					ver, err := parseVersion(versionStr)
					if err != nil {
						return xerrors.Errorf("parse --version: %w", err)
					}
					changed, err := updateReleaseDocs(ver, channel)
					if err != nil {
						return err
					}
					for _, f := range changed {
						_, _ = fmt.Fprintln(inv.Stdout, f)
					}
					return nil
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
