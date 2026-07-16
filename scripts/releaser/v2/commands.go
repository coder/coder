package v2

import (
	"fmt"
	"os"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

const (
	owner = "coder"
	repo  = "coder"
)

// newExecutor returns the appropriate CommandExecutor based on the
// dry-run setting.
//
//nolint:revive // dryRun selects the dry-run executor.
func newExecutor(dryRun bool) CommandExecutor {
	if dryRun {
		return newDryRunExecutor(os.Stderr)
	}
	return realExecutor{}
}

// dryRunOption returns the shared --dry-run option bound to dryRun.
func dryRunOption(dryRun *bool) serpent.Option {
	return serpent.Option{
		Name:        "dry-run",
		Flag:        "dry-run",
		Description: "Print mutating commands instead of executing them.",
		Value:       serpent.BoolOf(dryRun),
	}
}

// CICommands returns the low-level, CI-oriented release subcommands
// (calculate-version, prepare-release, generate-notes, publish). Their
// names, flags, and stdout output match the former scripts/release-action
// tool so GitHub Actions workflows can invoke them unchanged.
func CICommands() []*serpent.Command {
	return []*serpent.Command{
		calculateVersionCommand(),
		prepareReleaseCommand(),
		generateNotesCommand(),
		publishCommand(),
	}
}

// TypeCommand returns a command that runs prepare-release for a fixed
// release type. It backs the top-level rc, branch, and release
// subcommands, printing the same JSON as prepare-release.
func TypeCommand(use, short, releaseType string) *serpent.Command {
	var (
		ref       string
		commitSHA string
		dryRun    bool
	)
	return &serpent.Command{
		Use:   use,
		Short: short,
		Options: serpent.OptionSet{
			{
				Name:        "ref",
				Flag:        "ref",
				Description: "Git ref (branch name) to release from.",
				Value:       serpent.StringOf(&ref),
				Required:    true,
			},
			{
				Name:        "commit",
				Flag:        "commit",
				Description: "Commit SHA to tag (defaults to HEAD of --ref if empty).",
				Value:       serpent.StringOf(&commitSHA),
			},
			dryRunOption(&dryRun),
		},
		Handler: func(inv *serpent.Invocation) error {
			result, err := prepareRelease(newExecutor(dryRun), releaseType, ref, commitSHA)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stdout, result.String())
			return nil
		},
	}
}

func calculateVersionCommand() *serpent.Command {
	var (
		releaseType string
		ref         string
		commitSHA   string
		dryRun      bool
	)
	return &serpent.Command{
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
			dryRunOption(&dryRun),
		},
		Handler: func(inv *serpent.Invocation) error {
			result, err := calculateNextVersion(newExecutor(dryRun), releaseType, ref, commitSHA)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stdout, result.String())
			return nil
		},
	}
}

func prepareReleaseCommand() *serpent.Command {
	var (
		releaseType string
		ref         string
		commitSHA   string
		dryRun      bool
	)
	return &serpent.Command{
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
			dryRunOption(&dryRun),
		},
		Handler: func(inv *serpent.Invocation) error {
			result, err := prepareRelease(newExecutor(dryRun), releaseType, ref, commitSHA)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stdout, result.String())
			return nil
		},
	}
}

func generateNotesCommand() *serpent.Command {
	var (
		versionStr     string
		prevVersionStr string
		dryRun         bool
	)
	return &serpent.Command{
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
			dryRunOption(&dryRun),
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
			notes, err := generateReleaseNotes(newExecutor(dryRun), newVer, prevVer)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(inv.Stdout, notes)
			return nil
		},
	}
}

func publishCommand() *serpent.Command {
	var (
		versionStr string
		stable     bool
		notesFile  string
		dryRun     bool
	)
	return &serpent.Command{
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
			dryRunOption(&dryRun),
		},
		Handler: func(inv *serpent.Invocation) error {
			assets := inv.Args
			if len(assets) == 0 {
				return xerrors.New("no asset files provided as arguments")
			}
			return publishRelease(newExecutor(dryRun), versionStr, stable, notesFile, assets)
		},
	}
}
