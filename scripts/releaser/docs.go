package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

const (
	calendarStartMarker = "<!-- RELEASE_CALENDAR_START -->"
	calendarEndMarker   = "<!-- RELEASE_CALENDAR_END -->"

	releasesFile     = "docs/install/releases/index.md"
	kubernetesFile   = "docs/install/kubernetes.md"
	rancherFile      = "docs/install/rancher.md"
	changelogURLFmt  = "https://coder.com/changelog/coder-%d-%d"
	releaseTagURLFmt = "https://github.com/coder/coder/releases/tag/%s"
)

// calendarRow represents one row in the release calendar table.
type calendarRow struct {
	// ReleaseName is the display name, e.g. "2.30" or
	// "[2.30](https://...)".
	ReleaseName string
	// Major and Minor parsed from the release name.
	Major int
	Minor int
	// ReleaseDate as displayed, e.g. "February 03, 2026".
	ReleaseDate string
	// Status like "Mainline", "Stable", "Not Supported", etc.
	Status string
	// LatestRelease as displayed, e.g.
	// "[v2.30.0](https://...)".
	LatestRelease string
}

var autoversionPragmaRe = regexp.MustCompile(
	`<!-- ?autoversion\(([^)]+)\): ?"([^"]+)" ?-->`,
)

// parseCalendarTable extracts calendar rows from the markdown
// between the start and end markers. Returns the rows and the
// column widths for re-rendering.
func parseCalendarTable(content string) ([]calendarRow, error) {
	startIdx := strings.Index(content, calendarStartMarker)
	endIdx := strings.Index(content, calendarEndMarker)
	if startIdx == -1 || endIdx == -1 {
		return nil, xerrors.New("calendar markers not found")
	}

	tableContent := content[startIdx+len(calendarStartMarker) : endIdx]
	lines := strings.Split(strings.TrimSpace(tableContent), "\n")

	var rows []calendarRow
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip header and separator lines.
		if strings.HasPrefix(line, "| Release") ||
			strings.HasPrefix(line, "|---") ||
			strings.HasPrefix(line, "|-") {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}

		cols := strings.Split(line, "|")
		// Split on "|" gives empty first and last elements.
		if len(cols) < 5 {
			continue
		}
		name := strings.TrimSpace(cols[1])
		date := strings.TrimSpace(cols[2])
		status := strings.TrimSpace(cols[3])
		latest := strings.TrimSpace(cols[4])

		major, minor := parseReleaseName(name)
		rows = append(rows, calendarRow{
			ReleaseName:   name,
			Major:         major,
			Minor:         minor,
			ReleaseDate:   date,
			Status:        status,
			LatestRelease: latest,
		})
	}

	if len(rows) == 0 {
		return nil, xerrors.New("no calendar rows found")
	}
	return rows, nil
}

// parseReleaseName extracts major.minor from a release name
// like "2.30" or "[2.30](https://...)".
func parseReleaseName(name string) (major, minor int) {
	// Strip markdown link if present.
	re := regexp.MustCompile(`\[(\d+\.\d+)\]`)
	if m := re.FindStringSubmatch(name); len(m) > 1 {
		name = m[1]
	}
	_, _ = fmt.Sscanf(name, "%d.%d", &major, &minor)
	return major, minor
}

// renderCalendarTable renders the calendar rows as a markdown
// table.
func renderCalendarTable(rows []calendarRow) string {
	// Compute column widths.
	nameW, dateW, statusW, latestW := 12, 12, 6, 14
	for _, r := range rows {
		if len(r.ReleaseName) > nameW {
			nameW = len(r.ReleaseName)
		}
		if len(r.ReleaseDate) > dateW {
			dateW = len(r.ReleaseDate)
		}
		if len(r.Status) > statusW {
			statusW = len(r.Status)
		}
		if len(r.LatestRelease) > latestW {
			latestW = len(r.LatestRelease)
		}
	}

	var b strings.Builder
	// Header.
	_, _ = fmt.Fprintf(&b, "| %-*s | %-*s | %-*s | %-*s |\n",
		nameW, "Release name",
		dateW, "Release Date",
		statusW, "Status",
		latestW, "Latest Release")
	// Separator.
	_, _ = fmt.Fprintf(&b, "|%s|%s|%s|%s|\n",
		strings.Repeat("-", nameW+1),
		strings.Repeat("-", dateW+2),
		strings.Repeat("-", statusW+2),
		strings.Repeat("-", latestW+2))
	// Data rows.
	for _, r := range rows {
		_, _ = fmt.Fprintf(&b, "| %-*s | %-*s | %-*s | %-*s |\n",
			nameW, r.ReleaseName,
			dateW, r.ReleaseDate,
			statusW, r.Status,
			latestW, r.LatestRelease)
	}
	return b.String()
}

// updateCalendar modifies the calendar rows based on the new
// release version and channel.
func updateCalendar(
	rows []calendarRow,
	newVer version,
	channel string,
) []calendarRow {
	// For any release, update the "Latest Release" for the
	// matching major.minor row.
	for i, r := range rows {
		if r.Major == newVer.Major && r.Minor == newVer.Minor {
			rows[i].LatestRelease = fmt.Sprintf(
				"[v%s](%s)",
				newVer.String(),
				fmt.Sprintf(releaseTagURLFmt, newVer.String()),
			)
			// If this row was "Not Released", update it.
			if r.Status == "Not Released" {
				rows[i].Status = "Mainline"
				rows[i].ReleaseDate = time.Now().Format("January 02, 2006")
				rows[i].ReleaseName = fmt.Sprintf(
					"[%d.%d](%s)",
					newVer.Major, newVer.Minor,
					fmt.Sprintf(changelogURLFmt, newVer.Major, newVer.Minor),
				)
			}
		}
	}

	// For patch releases, we only update Latest Release — done
	// above.
	if newVer.Patch > 0 {
		return rows
	}

	// For new mainline releases (patch == 0), apply status
	// transitions.
	if channel == "mainline" {
		for i, r := range rows {
			switch {
			case r.Major == newVer.Major && r.Minor == newVer.Minor:
				// Already handled above.
				continue
			case r.Status == "Mainline":
				rows[i].Status = "Stable"
			case strings.Contains(r.Status, "Stable"):
				// "Stable", "Stable + ESR" → Security Support.
				rows[i].Status = "Security Support"
			case r.Status == "Security Support":
				rows[i].Status = "Not Supported"
			}
		}

		// Add "Not Released" row for the next minor.
		nextMinor := newVer.Minor + 1
		hasNext := false
		for _, r := range rows {
			if r.Major == newVer.Major && r.Minor == nextMinor {
				hasNext = true
				break
			}
		}
		if !hasNext {
			rows = append(rows, calendarRow{
				ReleaseName:   fmt.Sprintf("%d.%d", newVer.Major, nextMinor),
				Major:         newVer.Major,
				Minor:         nextMinor,
				ReleaseDate:   "",
				Status:        "Not Released",
				LatestRelease: "N/A",
			})
		}

		// Trim oldest "Not Supported" rows to keep roughly
		// the same number of rows. We allow up to the
		// current count + 1 (for the new "Not Released"
		// row), then trim.
		rows = trimOldestNotSupported(rows)
	}

	return rows
}

// trimOldestNotSupported removes "Not Supported" rows from the
// start until we have at most 8 rows total, keeping at least
// one "Not Supported" row if any exist.
func trimOldestNotSupported(rows []calendarRow) []calendarRow {
	const maxRows = 8
	for len(rows) > maxRows {
		// Find the first "Not Supported" row.
		found := -1
		for i, r := range rows {
			if r.Status == "Not Supported" {
				found = i
				break
			}
		}
		if found == -1 {
			break
		}
		// Count how many "Not Supported" rows we have.
		nsCount := 0
		for _, r := range rows {
			if r.Status == "Not Supported" {
				nsCount++
			}
		}
		// Keep at least one.
		if nsCount <= 1 {
			break
		}
		rows = append(rows[:found], rows[found+1:]...)
	}
	return rows
}

// updateCalendarFile reads the releases index.md, updates the
// calendar table, and writes it back.
func updateCalendarFile(
	repoRoot string,
	newVer version,
	channel string,
) error {
	path := filepath.Join(repoRoot, releasesFile)
	content, err := os.ReadFile(path)
	if err != nil {
		return xerrors.Errorf("reading %s: %w", releasesFile, err)
	}

	rows, err := parseCalendarTable(string(content))
	if err != nil {
		return xerrors.Errorf("parsing calendar: %w", err)
	}

	rows = updateCalendar(rows, newVer, channel)
	newTable := renderCalendarTable(rows)

	// Replace the content between markers.
	s := string(content)
	startIdx := strings.Index(s, calendarStartMarker)
	endIdx := strings.Index(s, calendarEndMarker)
	updated := s[:startIdx+len(calendarStartMarker)] +
		"\n" + newTable +
		s[endIdx:]

	//nolint:gosec // File permissions match the original.
	return os.WriteFile(path, []byte(updated), 0o644)
}

// updateAutoversionFile reads a markdown file and replaces
// version strings in lines following autoversion pragmas for
// the given channel.
func updateAutoversionFile(path, channel, newVer string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return xerrors.Errorf("reading %s: %w", path, err)
	}

	lines := strings.Split(string(content), "\n")
	changed := false

	for i, line := range lines {
		m := autoversionPragmaRe.FindStringSubmatch(line)
		if len(m) < 3 {
			continue
		}
		pragmaChannel := m[1]
		pattern := m[2]

		if pragmaChannel != channel {
			continue
		}

		// Build regex from the pattern by replacing
		// [version] with a capture group.
		escaped := regexp.QuoteMeta(pattern)
		reStr := strings.ReplaceAll(
			escaped,
			regexp.QuoteMeta("[version]"),
			`(\d+\.\d+\.\d+)`,
		)
		re, err := regexp.Compile(reStr)
		if err != nil {
			continue
		}

		// Search the next few lines for a match.
		for j := i + 1; j < len(lines) && j <= i+5; j++ {
			if loc := re.FindStringSubmatchIndex(lines[j]); loc != nil {
				// loc[2]:loc[3] is the version capture
				// group.
				lines[j] = lines[j][:loc[2]] + newVer + lines[j][loc[3]:]
				changed = true
				break
			}
		}
	}

	if !changed {
		return nil
	}

	//nolint:gosec // File permissions match the original.
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// updateRancherFile updates the version strings in rancher.md.
func updateRancherFile(path, channel, newVer string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return xerrors.Errorf("reading %s: %w", path, err)
	}

	s := string(content)

	switch channel {
	case "mainline":
		// Match: - **Mainline**: `X.Y.Z`
		re := regexp.MustCompile(
			`(\*\*Mainline\*\*: ` + "`)" + `\d+\.\d+\.\d+` + "(`)",
		)
		s = re.ReplaceAllString(s, "${1}"+newVer+"${2}")
	case "stable":
		re := regexp.MustCompile(
			`(\*\*Stable\*\*: ` + "`)" + `\d+\.\d+\.\d+` + "(`)",
		)
		s = re.ReplaceAllString(s, "${1}"+newVer+"${2}")
	default:
		return nil
	}

	//nolint:gosec // File permissions match the original.
	return os.WriteFile(path, []byte(s), 0o644)
}

// updateReleaseDocs updates all release-related docs files and
// creates a PR with the changes.
//
//nolint:revive // dryRun flag is needed to control PR creation behavior.
func updateReleaseDocs(
	inv *serpent.Invocation,
	newVer version,
	channel string,
	dryRun bool,
) error {
	w := inv.Stderr

	// Find the repo root (where .git is).
	repoRoot, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return xerrors.Errorf("finding repo root: %w", err)
	}

	verStr := fmt.Sprintf("%d.%d.%d", newVer.Major, newVer.Minor, newVer.Patch)
	vTag := "v" + verStr
	branchName := fmt.Sprintf("docs/update-release-%s", vTag)

	infof(w, "Updating release docs for %s (channel: %s)...", vTag, channel)
	fmt.Fprintln(w)

	if dryRun {
		_, _ = fmt.Fprintf(w, "[DRYRUN] would update %s\n", releasesFile)
		_, _ = fmt.Fprintf(w, "[DRYRUN] would update %s\n", kubernetesFile)
		_, _ = fmt.Fprintf(w, "[DRYRUN] would update %s\n", rancherFile)
		_, _ = fmt.Fprintf(w, "[DRYRUN] would create branch %s\n", branchName)
		_, _ = fmt.Fprintf(w, "[DRYRUN] would create PR: chore(docs): update release docs for %s\n", vTag)
		return nil
	}

	// Create a new branch from main.
	if err := gitRun("checkout", "-b", branchName, "origin/main"); err != nil {
		return xerrors.Errorf("creating branch: %w", err)
	}

	// Update the files.
	if err := updateCalendarFile(repoRoot, newVer, channel); err != nil {
		return xerrors.Errorf("updating calendar: %w", err)
	}
	successf(w, "Updated %s", releasesFile)

	k8sPath := filepath.Join(repoRoot, kubernetesFile)
	if err := updateAutoversionFile(k8sPath, channel, verStr); err != nil {
		return xerrors.Errorf("updating kubernetes.md: %w", err)
	}
	successf(w, "Updated %s", kubernetesFile)

	rancherPath := filepath.Join(repoRoot, rancherFile)
	if err := updateRancherFile(rancherPath, channel, verStr); err != nil {
		return xerrors.Errorf("updating rancher.md: %w", err)
	}
	successf(w, "Updated %s", rancherFile)

	// Stage and commit.
	if err := gitRun("add",
		filepath.Join(repoRoot, releasesFile),
		k8sPath,
		rancherPath,
	); err != nil {
		return xerrors.Errorf("staging files: %w", err)
	}

	commitMsg := fmt.Sprintf("chore(docs): update release docs for %s", vTag)
	if err := gitRun("commit", "-m", commitMsg); err != nil {
		return xerrors.Errorf("committing: %w", err)
	}

	// Push and create PR.
	if err := gitRun("push", "origin", branchName); err != nil {
		return xerrors.Errorf("pushing branch: %w", err)
	}

	prTitle := commitMsg
	prBody := fmt.Sprintf("Automated docs update for %s release.\n\nCreated by `releasetui`.", vTag)

	out, err := ghOutput("pr", "create",
		"--repo", owner+"/"+repo,
		"--title", prTitle,
		"--body", prBody,
		"--base", "main",
		"--head", branchName,
	)
	if err != nil {
		return xerrors.Errorf("creating PR: %w", err)
	}

	prURL := strings.TrimSpace(out)
	successf(w, "Created PR: %s", prURL)
	fmt.Fprintln(w)
	infof(w, "Review and merge the PR to complete the docs update.")

	return nil
}

// promptAndUpdateDocs asks the user if they want to create a
// docs update PR and does so if confirmed.
func promptAndUpdateDocs(
	inv *serpent.Invocation,
	newVer version,
	channel string,
	dryRun bool,
) {
	w := inv.Stderr
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, pretty.Sprint(cliui.BoldFmt(),
		"Next step: create a PR updating release docs "+
			"(calendar, helm versions, rancher)."))
	_, _ = fmt.Fprintln(w)

	if err := confirmWithDefault(inv, "Create docs update PR?", cliui.ConfirmYes); err != nil {
		infof(w, "Skipped docs update. You can update them manually.")
		return
	}

	if err := updateReleaseDocs(inv, newVer, channel, dryRun); err != nil {
		warnf(w, "Failed to create docs PR: %v", err)
		warnf(w, "You'll need to update release docs manually.")
	}
}
