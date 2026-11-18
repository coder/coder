package filefinder_test

import (
	"testing"

	"github.com/coder/coder/v2/agent/filefinder"
)

func TestIndex_AddAndLen(t *testing.T) {
	t.Parallel()
	idx := filefinder.NewIndex()
	idx.Add("foo/bar.go", 0)
	idx.Add("foo/baz.go", 0)
	if idx.Len() != 2 {
		t.Fatalf("expected 2, got %d", idx.Len())
	}
}

func TestIndex_Has(t *testing.T) {
	t.Parallel()
	idx := filefinder.NewIndex()
	idx.Add("foo/bar.go", 0)
	if !idx.Has("foo/bar.go") {
		t.Fatal("expected Has to return true")
	}
	if idx.Has("foo/missing.go") {
		t.Fatal("expected Has to return false for missing path")
	}
}

func TestIndex_Remove(t *testing.T) {
	t.Parallel()
	idx := filefinder.NewIndex()
	idx.Add("foo/bar.go", 0)
	if !idx.Remove("foo/bar.go") {
		t.Fatal("expected Remove to return true")
	}
	if idx.Has("foo/bar.go") {
		t.Fatal("expected Has to return false after Remove")
	}
	if idx.Len() != 0 {
		t.Fatalf("expected Len 0 after Remove, got %d", idx.Len())
	}
}

func TestIndex_RemoveNonexistent(t *testing.T) {
	t.Parallel()
	idx := filefinder.NewIndex()
	if idx.Remove("nonexistent.go") {
		t.Fatal("expected Remove to return false for missing path")
	}
}

func TestIndex_PathNormalization(t *testing.T) {
	t.Parallel()
	idx := filefinder.NewIndex()
	idx.Add("Foo/Bar.go", 0)
	// Should be findable with lowercase.
	if !idx.Has("foo/bar.go") {
		t.Fatal("expected case-insensitive Has")
	}
}
