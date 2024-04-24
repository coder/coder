package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v61/github"
	"github.com/spf13/afero"
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
	// Pre-flight checks.
	toplevel, err := run("git", "rev-parse", "--show-toplevel")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		_, _ = fmt.Fprintf(os.Stderr, "NOTE: This command must be run in the coder/coder repository.\n")
		os.Exit(1)
	}

	if err = checkCoderRepo(toplevel); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		_, _ = fmt.Fprintf(os.Stderr, "NOTE: This command must be run in the coder/coder repository.\n")
		os.Exit(1)
	}

	r := &releaseCommand{
		fs:     afero.NewBasePathFs(afero.NewOsFs(), toplevel),
		logger: slog.Make(sloghuman.Sink(os.Stderr)).Leveled(slog.LevelInfo),
	}

	var channel string

	cmd := serpent.Command{
		Use:   "release <subcommand>",
		Short: "Prepare, create and publish releases.",
		Options: serpent.OptionSet{
			{
				Flag:        "debug",
				Description: "Enable debug logging.",
				Value:       serpent.BoolOf(&r.debug),
			},
			{
				Flag:        "gh-token",
				Description: "GitHub personal access token.",
				Env:         "GH_TOKEN",
				Value:       serpent.StringOf(&r.ghToken),
			},
			{
				Flag:          "dry-run",
				FlagShorthand: "n",
				Description:   "Do not make any changes, only print what would be done.",
				Value:         serpent.BoolOf(&r.dryRun),
			},
		},
		Children: []*serpent.Command{
			{
				Use:        "promote <version>",
				Short:      "Promote version to stable.",
				Middleware: r.debugMiddleware, // Serpent doesn't support this on parent.
				Handler: func(inv *serpent.Invocation) error {
					ctx := inv.Context()
					if len(inv.Args) == 0 {
						return xerrors.New("version argument missing")
					}
					if !r.dryRun && r.ghToken == "" {
						return xerrors.New("GitHub personal access token is required, use --gh-token or GH_TOKEN")
					}

					err := r.promoteVersionToStable(ctx, inv, inv.Args[0])
					if err != nil {
						return err
					}

					return nil
				},
			},
			{
				Use:   "autoversion <version>",
				Short: "Automatically update the provided channel to version in markdown files.",
				Options: serpent.OptionSet{
					{
						Flag:        "channel",
						Description: "Channel to update.",
						Value:       serpent.EnumOf(&channel, "mainline", "stable"),
					},
				},
				Middleware: r.debugMiddleware, // Serpent doesn't support this on parent.
				Handler: func(inv *serpent.Invocation) error {
					ctx := inv.Context()
					if len(inv.Args) == 0 {
						return xerrors.New("version argument missing")
					}

					err := r.autoversion(ctx, channel, inv.Args[0])
					if err != nil {
						return err
					}

					return nil
				},
			},
		},
	}

	err = cmd.Invoke().WithOS().Run()
	if err != nil {
		if errors.Is(err, cliui.Canceled) {
			os.Exit(1)
		}
		r.logger.Error(context.Background(), "release command failed", "err", err)
		os.Exit(1)
	}
}

func checkCoderRepo(path string) error {
	remote, err := run("git", "-C", path, "remote", "get-url", "origin")
	if err != nil {
		return xerrors.Errorf("get remote failed: %w", err)
	}
	if !strings.Contains(remote, "github.com") || !strings.Contains(remote, "coder/coder") {
		return xerrors.Errorf("origin is not set to the coder/coder repository on github.com")
	}
	return nil
}

type releaseCommand struct {
	fs      afero.Fs
	logger  slog.Logger
	debug   bool
	ghToken string
	dryRun  bool
}

func (r *releaseCommand) debugMiddleware(next serpent.HandlerFunc) serpent.HandlerFunc {
	return func(inv *serpent.Invocation) error {
		if r.debug {
			r.logger = r.logger.Leveled(slog.LevelDebug)
		}
		if r.dryRun {
			r.logger = r.logger.With(slog.F("dry_run", true))
		}
		return next(inv)
	}
}

//nolint:revive // Allow dryRun control flag.
func (r *releaseCommand) promoteVersionToStable(ctx context.Context, inv *serpent.Invocation, version string) error {
	client := github.NewClient(nil)
	if r.ghToken != "" {
		client = client.WithAuthToken(r.ghToken)
	}

	logger := r.logger.With(slog.F("version", version))

	logger.Info(ctx, "checking current stable release")

	// Check if the version is already the latest stable release.
	currentStable, _, err := client.Repositories.GetLatestRelease(ctx, "coder", "coder")
	if err != nil {
		return xerrors.Errorf("get latest release failed: %w", err)
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
		return xerrors.Errorf("list releases failed: %w", err)
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

	updatedBody := removeMainlineBlurb(newStable.GetBody())
	updatedBody = addStableSince(time.Now().UTC(), updatedBody)
	updatedNewStable.Body = github.String(updatedBody)
	updatedNewStable.Prerelease = github.Bool(false)
	updatedNewStable.Draft = github.Bool(false)
	if !r.dryRun {
		_, _, err = client.Repositories.EditRelease(ctx, owner, repo, newStable.GetID(), newStable)
		if err != nil {
			return xerrors.Errorf("edit release failed: %w", err)
		}
		logger.Info(ctx, "selected version promoted to stable", "url", newStable.GetHTMLURL())
	} else {
		logger.Info(ctx, "dry-run: release not updated", "uncommitted_changes", cmp.Diff(newStable, updatedNewStable))
	}

	return nil
}

func cloneRelease(r *github.RepositoryRelease) *github.RepositoryRelease {
	rr := *r
	return &rr
}

// addStableSince adds a stable since note to the release body.
//
// Example:
//
//	> ## Stable (since April 23, 2024)
func addStableSince(date time.Time, body string) string {
	return fmt.Sprintf("> ## Stable (since %s)\n\n", date.Format("January 02, 2006")) + body
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

// autoversion automatically updates the provided channel to version in markdown
// files.
func (r *releaseCommand) autoversion(ctx context.Context, channel, version string) error {
	var files []string

	// For now, scope this to docs, perhaps we include README.md in the future.
	if err := afero.Walk(r.fs, "docs", func(path string, _ fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return xerrors.Errorf("walk failed: %w", err)
	}

	for _, file := range files {
		err := r.autoversionFile(ctx, file, channel, version)
		if err != nil {
			return xerrors.Errorf("autoversion file failed: %w", err)
		}
	}

	return nil
}

// autoversionMarkdownPragmaRe matches the autoversion pragma in markdown files.
//
// Example:
//
//	<!-- autoversion(stable): "--version [version]" -->
//
// The channel is the first capture group and the match string is the second
// capture group. The string "[version]" is replaced with the new version.
var autoversionMarkdownPragmaRe = regexp.MustCompile(`<!-- ?autoversion\(([^)]+)\): ?"([^"]+)" ?-->`)

func (r *releaseCommand) autoversionFile(ctx context.Context, file, channel, version string) error {
	version = strings.TrimPrefix(version, "v")
	logger := r.logger.With(slog.F("file", file), slog.F("channel", channel), slog.F("version", version))

	logger.Debug(ctx, "checking file for autoversion pragma")

	contents, err := afero.ReadFile(r.fs, file)
	if err != nil {
		return xerrors.Errorf("read file failed: %w", err)
	}

	lines := strings.Split(string(contents), "\n")
	var matchRe *regexp.Regexp
	for i, line := range lines {
		if autoversionMarkdownPragmaRe.MatchString(line) {
			matches := autoversionMarkdownPragmaRe.FindStringSubmatch(line)
			matchChannel := matches[1]
			match := matches[2]

			logger := logger.With(slog.F("line_number", i+1), slog.F("match_channel", matchChannel), slog.F("match", match))

			logger.Debug(ctx, "autoversion pragma detected")

			if matchChannel != channel {
				logger.Debug(ctx, "channel mismatch, skipping")
				continue
			}

			logger.Info(ctx, "autoversion pragma found with channel match")

			match = strings.Replace(match, "[version]", `(?P<version>[0-9]+\.[0-9]+\.[0-9]+)`, 1)
			logger.Debug(ctx, "compiling match regexp", "match", match)
			matchRe, err = regexp.Compile(match)
			if err != nil {
				return xerrors.Errorf("regexp compile failed: %w", err)
			}
		}
		if matchRe != nil {
			// Apply matchRe and find the group named "version", then replace it with the new version.
			// Utilize the index where the match was found to replace the correct part. The only
			// match group is the version.
			if match := matchRe.FindStringSubmatchIndex(line); match != nil {
				logger.Info(ctx, "updating version number", "line_number", i+1, "match", match)
				lines[i] = line[:match[2]] + version + line[match[3]:]
				matchRe = nil
				break
			}
		}
	}
	if matchRe != nil {
		return xerrors.Errorf("match not found in file")
	}

	updated := strings.Join(lines, "\n")

	// Only update the file if there are changes.
	diff := cmp.Diff(string(contents), updated)
	if diff == "" {
		return nil
	}

	if !r.dryRun {
		if err := afero.WriteFile(r.fs, file, []byte(updated), 0o644); err != nil {
			return xerrors.Errorf("write file failed: %w", err)
		}
		logger.Info(ctx, "file autoversioned")
	} else {
		logger.Info(ctx, "dry-run: file not updated", "uncommitted_changes", diff)
	}

	return nil
}

func run(command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", xerrors.Errorf("command failed: %q: %w\n%s", fmt.Sprintf("%s %s", command, strings.Join(args, " ")), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}
