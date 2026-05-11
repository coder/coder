package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

// calendarRow represents a single row in the release calendar table.
type calendarRow struct {
	ReleaseName   string
	ReleaseDate   string
	Status        string
	LatestRelease string
}

// updateReleaseDocs updates release calendar and version files on disk
// and returns the list of changed file paths. Skips if the version is
// an RC.
func updateReleaseDocs(ver version, channel string) ([]string, error) {
	if ver.IsRC() {
		return nil, nil
	}

	var changed []string

	// Update the release calendar.
	calendarPath := "docs/install/releases/index.md"
	if err := updateCalendarFile(calendarPath, ver); err != nil {
		return nil, xerrors.Errorf("update calendar: %w", err)
	}
	changed = append(changed, calendarPath)

	// Update Kubernetes autoversion file.
	k8sPath := "docs/install/kubernetes.md"
	if err := updateAutoversionFile(k8sPath, ver, channel); err != nil {
		return nil, xerrors.Errorf("update kubernetes docs: %w", err)
	}
	changed = append(changed, k8sPath)

	// Update Rancher file.
	rancherPath := "docs/install/rancher.md"
	if err := updateRancherFile(rancherPath, ver, channel); err != nil {
		return nil, xerrors.Errorf("update rancher docs: %w", err)
	}
	changed = append(changed, rancherPath)

	return changed, nil
}

var (
	calendarStartMarker = "<!-- RELEASE_CALENDAR_START -->"
	calendarEndMarker   = "<!-- RELEASE_CALENDAR_END -->"
)

// updateCalendarFile reads the calendar file, updates the table, and
// writes it back.
func updateCalendarFile(path string, ver version) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return xerrors.Errorf("read %s: %w", path, err)
	}

	content := string(data)
	updated, err := updateCalendar(content, ver)
	if err != nil {
		return err
	}

	return os.WriteFile(path, []byte(updated), 0o644)
}

// updateCalendar updates the release calendar table within the given
// markdown content.
func updateCalendar(content string, ver version) (string, error) {
	startIdx := strings.Index(content, calendarStartMarker)
	endIdx := strings.Index(content, calendarEndMarker)
	if startIdx < 0 || endIdx < 0 {
		return "", xerrors.New("calendar markers not found in content")
	}

	// Extract the table between markers.
	tableStart := startIdx + len(calendarStartMarker) + 1
	tableContent := content[tableStart:endIdx]

	rows, err := parseCalendarTable(tableContent)
	if err != nil {
		return "", xerrors.Errorf("parse calendar: %w", err)
	}

	releaseName := fmt.Sprintf("%d.%d", ver.major, ver.minor)
	tagStr := ver.String()

	// Find the row for this release and update it.
	found := false
	for i, row := range rows {
		if parseReleaseName(row.ReleaseName) == releaseName {
			found = true
			rows[i].Status = "Stable"
			if row.ReleaseDate == "" {
				rows[i].ReleaseDate = time.Now().UTC().Format("January 02, 2006")
			}
			rows[i].LatestRelease = fmt.Sprintf("[%s](https://github.com/%s/%s/releases/tag/%s)", tagStr, owner, repo, tagStr)
			break
		}
	}

	if !found {
		// If the release row doesn't exist, update the "Not Released"
		// row if its minor matches.
		for i, row := range rows {
			if row.Status == "Not Released" {
				name := parseReleaseName(row.ReleaseName)
				if name == releaseName || name == "" {
					rows[i].ReleaseName = fmt.Sprintf("[%s](https://coder.com/changelog/coder-%d-%d)", releaseName, ver.major, ver.minor)
					rows[i].ReleaseDate = time.Now().UTC().Format("January 02, 2006")
					rows[i].Status = "Stable"
					rows[i].LatestRelease = fmt.Sprintf("[%s](https://github.com/%s/%s/releases/tag/%s)", tagStr, owner, repo, tagStr)
					found = true
					break
				}
			}
		}
	}

	if !found {
		return "", xerrors.Errorf("no matching row found for release %s", releaseName)
	}

	// Trim oldest not-supported entries.
	rows = trimOldestNotSupported(rows)

	// Re-render the table.
	newTable := renderCalendarTable(rows)

	return content[:tableStart] + newTable + content[endIdx:], nil
}

// parseCalendarTable parses the markdown table rows between the
// calendar markers. Skips the header and separator rows.
func parseCalendarTable(table string) ([]calendarRow, error) {
	lines := strings.Split(strings.TrimSpace(table), "\n")

	var rows []calendarRow
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip header row and separator.
		if i < 2 {
			continue
		}

		cols := splitTableRow(line)
		if len(cols) < 4 {
			continue
		}

		rows = append(rows, calendarRow{
			ReleaseName:   strings.TrimSpace(cols[0]),
			ReleaseDate:   strings.TrimSpace(cols[1]),
			Status:        strings.TrimSpace(cols[2]),
			LatestRelease: strings.TrimSpace(cols[3]),
		})
	}

	return rows, nil
}

// splitTableRow splits a markdown table row by pipes, ignoring the
// leading and trailing empty fields.
func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// renderCalendarTable renders the calendar rows back into a markdown
// table.
func renderCalendarTable(rows []calendarRow) string {
	// Calculate column widths.
	headers := []string{"Release name", "Release Date", "Status", "Latest Release"}
	widths := make([]int, 4)
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		cols := []string{row.ReleaseName, row.ReleaseDate, row.Status, row.LatestRelease}
		for i, c := range cols {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}

	var b strings.Builder

	// Header.
	fmt.Fprintf(&b, "| %-*s | %-*s | %-*s | %-*s |\n",
		widths[0], headers[0],
		widths[1], headers[1],
		widths[2], headers[2],
		widths[3], headers[3])

	// Separator.
	fmt.Fprintf(&b, "|%s|%s|%s|%s|\n",
		strings.Repeat("-", widths[0]+2),
		strings.Repeat("-", widths[1]+2),
		strings.Repeat("-", widths[2]+2),
		strings.Repeat("-", widths[3]+2))

	// Rows.
	for _, row := range rows {
		fmt.Fprintf(&b, "| %-*s | %-*s | %-*s | %-*s |\n",
			widths[0], row.ReleaseName,
			widths[1], row.ReleaseDate,
			widths[2], row.Status,
			widths[3], row.LatestRelease)
	}

	return b.String()
}

// trimOldestNotSupported removes the oldest "Not Supported" rows,
// keeping at most 2.
func trimOldestNotSupported(rows []calendarRow) []calendarRow {
	const maxNotSupported = 2
	count := 0
	for _, r := range rows {
		if r.Status == "Not Supported" {
			count++
		}
	}

	if count <= maxNotSupported {
		return rows
	}

	toRemove := count - maxNotSupported
	var result []calendarRow
	for _, r := range rows {
		if r.Status == "Not Supported" && toRemove > 0 {
			toRemove--
			continue
		}
		result = append(result, r)
	}
	return result
}

// parseReleaseName extracts the bare "X.Y" from a release name cell
// which may be a markdown link like "[2.21](https://...)".
var releaseNameRe = regexp.MustCompile(`(\d+\.\d+)`)

func parseReleaseName(s string) string {
	m := releaseNameRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// updateAutoversionFile updates autoversion pragmas in a file for the
// given channel.
func updateAutoversionFile(path string, ver version, channel string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return xerrors.Errorf("read %s: %w", path, err)
	}

	versionStr := strings.TrimPrefix(ver.String(), "v")
	content := string(data)
	updated, err := applyAutoversion(content, versionStr, channel)
	if err != nil {
		return err
	}

	if updated == content {
		return nil
	}

	return os.WriteFile(path, []byte(updated), 0o644)
}

// autoversionPragmaRe matches autoversion pragmas in markdown.
var autoversionPragmaRe = regexp.MustCompile(`<!-- ?autoversion\(([^)]+)\): ?"([^"]+)" ?-->`)

// applyAutoversion processes autoversion pragmas in content.
func applyAutoversion(content, versionStr, channel string) (string, error) {
	lines := strings.Split(content, "\n")
	var matchRe *regexp.Regexp

	for i, line := range lines {
		if autoversionPragmaRe.MatchString(line) {
			matches := autoversionPragmaRe.FindStringSubmatch(line)
			matchChannel := matches[1]
			match := matches[2]

			if matchChannel != channel {
				continue
			}

			match = strings.Replace(match, "[version]", `(?P<version>[0-9]+\.[0-9]+\.[0-9]+)`, 1)
			var err error
			matchRe, err = regexp.Compile(match)
			if err != nil {
				return "", xerrors.Errorf("regexp compile failed: %w", err)
			}
		}
		if matchRe != nil {
			if match := matchRe.FindStringSubmatchIndex(line); match != nil {
				vg := matchRe.SubexpIndex("version")
				if vg == -1 {
					return "", xerrors.New("version group not found in match")
				}
				start := match[vg*2]
				end := match[vg*2+1]
				lines[i] = line[:start] + versionStr + line[end:]
				matchRe = nil
			}
		}
	}

	return strings.Join(lines, "\n"), nil
}

// updateRancherFile updates autoversion pragmas in the rancher docs.
func updateRancherFile(path string, ver version, channel string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return xerrors.Errorf("read %s: %w", path, err)
	}

	versionStr := strings.TrimPrefix(ver.String(), "v")
	content := string(data)
	updated, err := applyAutoversion(content, versionStr, channel)
	if err != nil {
		return err
	}

	if updated == content {
		return nil
	}

	return os.WriteFile(path, []byte(updated), 0o644)
}
