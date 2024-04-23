package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v43/github"
	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

const (
	owner = "coder"
	repo  = "coder"
)

func main() {
	logger := slog.Make(sloghuman.Sink(os.Stderr)).Leveled(slog.LevelDebug)

	var dryRun bool

	cmd := serpent.Command{
		Use:   "release <subcommand>",
		Short: "Prepare, create and publish releases.",
		Options: serpent.OptionSet{
			{
				Flag:          "dry-run",
				FlagShorthand: "n",
				Description:   "Do not make any changes, only print what would be done.",
				Value:         serpent.BoolOf(&dryRun),
			},
		},
		Children: []*serpent.Command{
			{
				Use:   "promote <version>",
				Short: "Promote version to stable.",
				Handler: func(inv *serpent.Invocation) error {
					ctx := inv.Context()
					if len(inv.Args) == 0 {
						return xerrors.New("version argument missing")
					}

					err := promoteVersionToStable(ctx, inv, logger, dryRun, inv.Args[0])
					if err != nil {
						return err
					}

					return nil
				},
			},
		},
	}

	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		if errors.Is(err, cliui.Canceled) {
			os.Exit(1)
		}
		logger.Error(context.Background(), "release command failed", "err", err)
		os.Exit(1)
	}
}

//nolint:revive // Allow dryRun control flag.
func promoteVersionToStable(ctx context.Context, inv *serpent.Invocation, logger slog.Logger, dryRun bool, version string) error {
	client := github.NewClient(nil)

	logger = logger.With(slog.F("dry_run", dryRun), slog.F("version", version))

	logger.Info(ctx, "checking current stable release")

	// Check if the version is already the latest stable release.
	currentStable, _, err := client.Repositories.GetLatestRelease(ctx, "coder", "coder")
	if err != nil {
		return err
	}

	logger = logger.With(slog.F("stable_version", currentStable.GetTagName()))
	logger.Info(ctx, "found current stable release")

	if currentStable.GetTagName() == version {
		return xerrors.Errorf("version %q is already the latest stable release", version)
	}

	// Ensure the version is a valid release.
	perPage := 20
	latestReleases, _, err := client.Repositories.ListReleases(ctx, owner, repo, &github.ListOptions{
		Page:    0,
		PerPage: perPage,
	})
	if err != nil {
		return err
	}

	var releaseVersions []string
	var newStable *github.RepositoryRelease
	for _, r := range latestReleases {
		releaseVersions = append(releaseVersions, r.GetTagName())
		if r.GetTagName() == version {
			newStable = r
		}
	}
	semver.Sort(releaseVersions)
	slices.Reverse(releaseVersions)

	switch {
	case len(releaseVersions) == 0:
		return xerrors.Errorf("no releases found")
	case newStable == nil:
		return xerrors.Errorf("version %q is not found in the last %d releases", version, perPage)
	}

	logger = logger.With(slog.F("mainline_version", releaseVersions[0]))

	if version != releaseVersions[0] {
		logger.Warn(ctx, "selected version is not the latest mainline release")
	}

	if reply, err := cliui.Prompt(inv, cliui.PromptOptions{
		Text:      "Are you sure you want to promote this version to stable?",
		Default:   "no",
		IsConfirm: true,
	}); err != nil {
		if reply == cliui.ConfirmNo {
			return nil
		}
		return err
	}

	logger.Info(ctx, "promoting selected version to stable")

	// Update the release to latest.
	updatedNewStable := cloneRelease(newStable)

	updatedNewStable.Name = github.String(newStable.GetName() + " (Stable)")
	updatedNewStable.Body = github.String(removeMainlineBlurb(newStable.GetBody()))
	updatedNewStable.Prerelease = github.Bool(false)
	updatedNewStable.Draft = github.Bool(false)
	if !dryRun {
		_, _, err = client.Repositories.EditRelease(ctx, owner, repo, newStable.GetID(), newStable)
		if err != nil {
			return err
		}
		logger.Info(ctx, "selected version promoted to stable")
	} else {
		logger.Info(ctx, "dry-run: release not updated", "uncommitted_changes", cmp.Diff(newStable, updatedNewStable))
	}

	logger.Info(ctx, "updating title of the previous stable release (remove stable suffix)")

	// Update the previous stable release to a regular release.
	updatedOldStable := cloneRelease(currentStable)
	currentStable.Name = github.String(strings.TrimSuffix(currentStable.GetName(), " (Stable)"))

	if !dryRun {
		_, _, err = client.Repositories.EditRelease(ctx, owner, repo, currentStable.GetID(), currentStable)
		if err != nil {
			return err
		}
		logger.Info(ctx, "title of the previous stable release updated")
	} else {
		logger.Info(ctx, "dry-run: previous release not updated", "uncommitted_changes", cmp.Diff(currentStable, updatedOldStable))
	}

	return nil
}

func cloneRelease(r *github.RepositoryRelease) *github.RepositoryRelease {
	rr := *r
	return &rr
}

// removeMainlineBlurb removes the mainline blurb from the release body.
//
// Example:
//
//	> [!NOTE]
//	> This is a mainline Coder release. We advise enterprise customers without a staging environment to install our [latest stable release](https://github.com/coder/coder/releases/latest) while we refine this version. Learn more about our [Release Schedule](https://coder.com/docs/v2/latest/install/releases).
func removeMainlineBlurb(body string) string {
	lines := strings.Split(body, "\n")

	var newBody, clip []string
	var found bool
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "> [!NOTE]") {
			clip = append(clip, line)
			found = true
			continue
		}
		if found {
			clip = append(clip, line)
			found = strings.HasPrefix(strings.TrimSpace(line), ">")
			continue
		}
		if !found && len(clip) > 0 {
			if !strings.Contains(strings.ToLower(strings.Join(clip, "\n")), "this is a mainline coder release") {
				newBody = append(newBody, clip...) // This is some other note, restore it.
			}
			clip = nil
		}
		newBody = append(newBody, line)
	}

	return strings.Join(newBody, "\n")
}
