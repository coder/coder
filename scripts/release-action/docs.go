package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/xerrors"
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
	ReleaseName   string
	Major         int
	Minor         int
	ReleaseDate   string
	Status        string
	LatestRelease string
}

var autoversionPragmaRe = regexp.MustCompile(
	`<!-- ?autoversion\(([^)]+)\): ?"([^"]+)" ?-->`,
)

// parseCalendarTable extracts calendar rows from the markdown
// between the start and end markers.
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
		if strings.HasPrefix(line, "| Release") ||
			strings.HasPrefix(line, "|---") ||
			strings.HasPrefix(line, "|-") {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}

		cols := strings.Split(line, "|")
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
	_, _ = fmt.Fprintf(&b, "| %-*s | %-*s | %-*s | %-*s |\n",
		nameW, "Release name",
		dateW, "Release Date",
		statusW, "Status",
		latestW, "Latest Release")
	_, _ = fmt.Fprintf(&b, "|%s|%s|%s|%s|\n",
		strings.Repeat("-", nameW+1),
		strings.Repeat("-", dateW+2),
		strings.Repeat("-", statusW+2),
		strings.Repeat("-", latestW+2))
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
	for i, r := range rows {
		if r.Major == newVer.Major && r.Minor == newVer.Minor {
			rows[i].LatestRelease = fmt.Sprintf(
				"[v%s](%s)",
				newVer.String(),
				fmt.Sprintf(releaseTagURLFmt, newVer.String()),
			)
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

	// For patch releases, only Latest Release changes.
	if newVer.Patch > 0 {
		return rows
	}

	// For new mainline releases (patch == 0), apply status
	// transitions.
	if channel == "mainline" {
		for i, r := range rows {
			switch {
			case r.Major == newVer.Major && r.Minor == newVer.Minor:
				continue
			case r.Status == "Mainline":
				rows[i].Status = "Stable"
			case strings.Contains(r.Status, "Stable"):
				rows[i].Status = "Security Support"
			case r.Status == "Security Support":
				rows[i].Status = "Not Supported"
			}
		}

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
		nsCount := 0
		for _, r := range rows {
			if r.Status == "Not Supported" {
				nsCount++
			}
		}
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

		for j := i + 1; j < len(lines) && j <= i+5; j++ {
			if loc := re.FindStringSubmatchIndex(lines[j]); loc != nil {
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
// returns the list of files that were modified. RC versions are
// skipped entirely.
func updateReleaseDocs(ver version, channel string) ([]string, error) {
	if ver.IsRC() {
		return nil, nil
	}

	repoRoot, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, xerrors.Errorf("finding repo root: %w", err)
	}

	verStr := fmt.Sprintf("%d.%d.%d", ver.Major, ver.Minor, ver.Patch)

	var changed []string

	if err := updateCalendarFile(repoRoot, ver, channel); err != nil {
		return nil, xerrors.Errorf("updating calendar: %w", err)
	}
	changed = append(changed, releasesFile)

	k8sPath := filepath.Join(repoRoot, kubernetesFile)
	if err := updateAutoversionFile(k8sPath, channel, verStr); err != nil {
		return nil, xerrors.Errorf("updating kubernetes.md: %w", err)
	}
	changed = append(changed, kubernetesFile)

	rancherPath := filepath.Join(repoRoot, rancherFile)
	if err := updateRancherFile(rancherPath, channel, verStr); err != nil {
		return nil, xerrors.Errorf("updating rancher.md: %w", err)
	}
	changed = append(changed, rancherFile)

	return changed, nil
}
