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
	// calculate-version flags
	var (
		releaseType string
		commitSHA   string
		branch      string
	)

	// generate-notes flags
	var (
		gnVersion     string
		gnPrevVersion string
		gnChannel     string
	)

	// update-docs flags
	var (
		udVersion string
		udChannel string
	)

	root := &serpent.Command{
		Use:   "release-action",
		Short: "Non-interactive, CI-oriented release helper for coder/coder.",
		Children: []*serpent.Command{
			{
				Use:   "calculate-version",
				Short: "Compute the next release version tag.",
				Options: serpent.OptionSet{
					{
						Name:        "type",
						Flag:        "type",
						Description: "Release type: rc, release, or create-release-branch.",
						Required:    true,
						Value:       serpent.StringOf(&releaseType),
					},
					{
						Name:        "commit",
						Flag:        "commit",
						Description: "Commit SHA to tag (RC and create-release-branch).",
						Value:       serpent.StringOf(&commitSHA),
					},
					{
						Name:        "branch",
						Flag:        "branch",
						Description: "Release branch name (release only, e.g. release/2.32).",
						Value:       serpent.StringOf(&branch),
					},
				},
				Handler: func(inv *serpent.Invocation) error {
					result, err := calculateNextVersion(releaseType, commitSHA, branch)
					if err != nil {
						return err
					}
					_, _ = fmt.Fprint(inv.Stdout, result)
					return nil
				},
			},
			{
				Use:   "generate-notes",
				Short: "Produce release notes markdown.",
				Options: serpent.OptionSet{
					{
						Name:        "version",
						Flag:        "version",
						Description: "New version tag (e.g. v2.32.0).",
						Required:    true,
						Value:       serpent.StringOf(&gnVersion),
					},
					{
						Name:        "previous-version",
						Flag:        "previous-version",
						Description: "Previous version tag for changelog range.",
						Required:    true,
						Value:       serpent.StringOf(&gnPrevVersion),
					},
					{
						Name:        "channel",
						Flag:        "channel",
						Description: "Release channel: rc, mainline, or stable.",
						Required:    true,
						Value:       serpent.StringOf(&gnChannel),
					},
				},
				Handler: func(inv *serpent.Invocation) error {
					newVer, ok := parseVersion(gnVersion)
					if !ok {
						return xerrors.Errorf("invalid version %q", gnVersion)
					}
					prevVer, ok := parseVersion(gnPrevVersion)
					if !ok {
						return xerrors.Errorf("invalid previous version %q", gnPrevVersion)
					}
					notes, err := generateReleaseNotes(newVer, prevVer, gnChannel)
					if err != nil {
						return err
					}
					_, _ = fmt.Fprint(inv.Stdout, notes)
					return nil
				},
			},
			{
				Use:   "update-docs",
				Short: "Modify docs files for calendar/autoversion.",
				Options: serpent.OptionSet{
					{
						Name:        "version",
						Flag:        "version",
						Description: "New version tag (e.g. v2.32.0).",
						Required:    true,
						Value:       serpent.StringOf(&udVersion),
					},
					{
						Name:        "channel",
						Flag:        "channel",
						Description: "Release channel: mainline or stable.",
						Required:    true,
						Value:       serpent.StringOf(&udChannel),
					},
				},
				Handler: func(inv *serpent.Invocation) error {
					ver, ok := parseVersion(udVersion)
					if !ok {
						return xerrors.Errorf("invalid version %q", udVersion)
					}
					changed, err := updateReleaseDocs(ver, udChannel)
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

	err := root.Invoke().WithOS().Run()
	if err != nil {
		var runErr *serpent.RunCommandError
		if errors.As(err, &runErr) {
			err = runErr.Err
		}
		_, _ = fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}
