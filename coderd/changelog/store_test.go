package changelog

import (
	"strings"
	"testing"

	"golang.org/x/mod/semver"
)

func TestStoreList(t *testing.T) {
	s := NewStore()
	entries, err := s.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("List() returned no entries")
	}

	for i := 1; i < len(entries); i++ {
		prev := entries[i-1]
		curr := entries[i]
		if prev.Date < curr.Date {
			t.Fatalf("entries not sorted by date desc at index %d: %q < %q", i, prev.Date, curr.Date)
		}
		if prev.Date == curr.Date {
			if semver.Compare("v"+prev.Version, "v"+curr.Version) < 0 {
				t.Fatalf("entries not sorted by version desc at index %d: %q < %q", i, prev.Version, curr.Version)
			}
		}
	}
}

func TestStoreGet(t *testing.T) {
	s := NewStore()
	entry, err := s.Get("2.30")
	if err != nil {
		t.Fatalf("Get(\"2.30\") error: %v", err)
	}

	if entry.Version != "2.30" {
		t.Fatalf("Version = %q, want %q", entry.Version, "2.30")
	}
	if entry.Title != "What's new in Coder 2.30" {
		t.Fatalf("Title = %q, want %q", entry.Title, "What's new in Coder 2.30")
	}
	if entry.Date != "2025-07-08" {
		t.Fatalf("Date = %q, want %q", entry.Date, "2025-07-08")
	}
	if entry.Summary != "Dynamic parameters, prebuilt workspaces, and more." {
		t.Fatalf("Summary = %q, want %q", entry.Summary, "Dynamic parameters, prebuilt workspaces, and more.")
	}
	if entry.Image != "assets/2.30-hero.webp" {
		t.Fatalf("Image = %q, want %q", entry.Image, "assets/2.30-hero.webp")
	}

	if strings.Contains(entry.Content, "version:") {
		t.Fatalf("Content unexpectedly contains frontmatter")
	}
	if !strings.HasPrefix(entry.Content, "## Dynamic Parameters") {
		t.Fatalf("Content does not start with expected heading; got %q", entry.Content[:min(50, len(entry.Content))])
	}
	for _, want := range []string{
		"## Dynamic Parameters",
		"## Prebuilt Workspaces",
		"## MCP Integration",
		"coder mcp server",
		"## Inbox Notifications",
	} {
		if !strings.Contains(entry.Content, want) {
			t.Fatalf("Content missing %q", want)
		}
	}
}

func TestStoreGet_NotFound(t *testing.T) {
	s := NewStore()
	entry, err := s.Get("99.99")
	if err == nil {
		t.Fatalf("Get(\"99.99\") expected error")
	}
	if entry != nil {
		t.Fatalf("Get(\"99.99\") entry = %#v, want nil", entry)
	}
	if !strings.Contains(err.Error(), "99.99") {
		t.Fatalf("error %q does not mention requested version", err.Error())
	}
}

func TestStoreHas(t *testing.T) {
	s := NewStore()
	if !s.Has("2.30") {
		t.Fatalf("Has(\"2.30\") = false, want true")
	}
	if s.Has("99.99") {
		t.Fatalf("Has(\"99.99\") = true, want false")
	}
}

func TestParseEntry(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		data := []byte(`---
version: "1.0"
title: "Title"
date: "2025-01-01"
summary: "Summary"
image: "assets/1.0.webp"
---

Hello world.
`)
		entry, err := parseEntry(data)
		if err != nil {
			t.Fatalf("parseEntry error: %v", err)
		}
		if entry.Version != "1.0" {
			t.Fatalf("Version = %q, want %q", entry.Version, "1.0")
		}
		if entry.Content != "Hello world." {
			t.Fatalf("Content = %q, want %q", entry.Content, "Hello world.")
		}
	})

	t.Run("missing_opening_delimiter", func(t *testing.T) {
		data := []byte("version: \"1.0\"\n---\nHello")
		_, err := parseEntry(data)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing_closing_delimiter", func(t *testing.T) {
		data := []byte("---\nversion: \"1.0\"\nHello")
		_, err := parseEntry(data)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing_version", func(t *testing.T) {
		data := []byte("---\ntitle: \"Title\"\n---\nHello")
		_, err := parseEntry(data)
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(err.Error(), "version") {
			t.Fatalf("error %q does not mention version", err.Error())
		}
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
