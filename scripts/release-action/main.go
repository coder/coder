package main

import (
	"errors"
	"fmt"
	"os"

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
		versionFlag     string
		prevVersionFlag string
		channelFlag     string
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
						Description: "Release type: rc or release.",
						Required:    true,
						Value:       serpent.StringOf(&releaseType),
					},
					{
						Name:        "commit",
						Flag:        "commit",
						Description: "Commit SHA to tag (RC only).",
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
						Value:       serpent.StringOf(&versionFlag),
					},
					{
						Name:        "previous-version",
						Flag:        "previous-version",
						Description: "Previous version tag for changelog range.",
						Required:    true,
						Value:       serpent.StringOf(&prevVersionFlag),
					},
					{
						Name:        "channel",
						Flag:        "channel",
						Description: "Release channel: rc, mainline, or stable.",
						Required:    true,
						Value:       serpent.StringOf(&channelFlag),
					},
				},
				Handler: func(inv *serpent.Invocation) error {
					newVer, ok := parseVersion(versionFlag)
					if !ok {
						return fmt.Errorf("invalid version %q", versionFlag)
					}
					prevVer, ok := parseVersion(prevVersionFlag)
					if !ok {
						return fmt.Errorf("invalid previous version %q", prevVersionFlag)
					}
					notes, err := generateReleaseNotes(newVer, prevVer, channelFlag)
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
						Value:       serpent.StringOf(&versionFlag),
					},
					{
						Name:        "channel",
						Flag:        "channel",
						Description: "Release channel: mainline or stable.",
						Required:    true,
						Value:       serpent.StringOf(&channelFlag),
					},
				},
				Handler: func(inv *serpent.Invocation) error {
					ver, ok := parseVersion(versionFlag)
					if !ok {
						return fmt.Errorf("invalid version %q", versionFlag)
					}
					changed, err := updateReleaseDocs(ver, channelFlag)
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
