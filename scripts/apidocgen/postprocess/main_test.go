package main

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

func TestPrependFrontMatter(t *testing.T) {
	t.Parallel()

	section := []byte("# Templates\n\nThe body.\n")
	got := string(prependFrontMatter(section, docgenenv.Route{
		Title:       "Templates",
		Description: "Manage templates",
	}))
	want := "---\ntitle: Templates\ndescription: Manage templates\n---\n\nThe body.\n"
	require.Equal(t, want, got)
}

// TestPrependFrontMatterStateAndQuoting covers the curated-metadata path: a
// description with characters YAML would misparse is quoted, and state renders
// as a YAML sequence.
func TestPrependFrontMatterStateAndQuoting(t *testing.T) {
	t.Parallel()

	section := []byte("# Chats\nBody starts immediately.\n")
	got := string(prependFrontMatter(section, docgenenv.Route{
		Title:       "Chats",
		Description: "REST endpoints for Coder Agents Chats API (programmatic agent sessions).",
		State:       []string{"early access"},
	}))
	want := "---\n" +
		"title: Chats\n" +
		`description: "REST endpoints for Coder Agents Chats API (programmatic agent sessions)."` + "\n" +
		"state:\n" +
		"  - early access\n" +
		"---\n\n" +
		"Body starts immediately.\n"
	require.Equal(t, want, got)
}

// TestPrependFrontMatterKeepsBodyWithoutHeading verifies the guard: when the
// first line is not the "# {name}" heading, the whole section is preserved
// rather than silently dropping the first content line.
func TestPrependFrontMatterKeepsBodyWithoutHeading(t *testing.T) {
	t.Parallel()

	section := []byte("No heading here.\nSecond line.\n")
	got := string(prependFrontMatter(section, docgenenv.Route{Title: "General"}))
	want := "---\ntitle: General\n---\n\nNo heading here.\nSecond line.\n"
	require.Equal(t, want, got)
}

// TestExtractSectionName covers extractSectionName's contract, the load-bearing
// guard prependFrontMatter relies on: the first line must be a "# {name}"
// heading, and a section without one (or an empty section) is rejected, not
// sliced into a bogus name.
func TestExtractSectionName(t *testing.T) {
	t.Parallel()

	name, err := extractSectionName([]byte("# Templates\n\nBody.\n"))
	require.NoError(t, err)
	require.Equal(t, "Templates", name)

	_, err = extractSectionName([]byte("Body without a heading.\n"))
	require.Error(t, err)

	_, err = extractSectionName(nil)
	require.Error(t, err)

	// A first line past bufio.Scanner's token limit makes Scan return false
	// with the reason only in Err(); surface it as a scanning error instead of
	// a missing-header error.
	_, err = extractSectionName([]byte(strings.Repeat("a", bufio.MaxScanTokenSize+1)))
	require.Error(t, err)
	require.Contains(t, err.Error(), "scanning section")
}
