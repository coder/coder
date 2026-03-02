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

func TestIndex_AddOverwrite(t *testing.T) {
	t.Parallel()
	idx := filefinder.NewIndex()
	idx.Add("foo/bar.go", uint16(filefinder.FlagFile))
	idx.Add("foo/bar.go", uint16(filefinder.FlagDir)) // overwrite
	if idx.Len() != 1 {
		t.Fatalf("expected 1 after overwrite, got %d", idx.Len())
	}
	// The old entry should be tombstoned.
	if !filefinder.IndexIsDeleted(idx, 0) {
		t.Fatal("expected old entry to be deleted")
	}
	if filefinder.IndexIsDeleted(idx, 1) {
		t.Fatal("expected new entry to be live")
	}
}

func TestIndex_Snapshot(t *testing.T) {
	t.Parallel()
	idx := filefinder.NewIndex()
	idx.Add("foo/bar.go", 0)
	idx.Add("foo/baz.go", 0)

	snap := idx.Snapshot()
	if filefinder.SnapshotCount(snap) != 2 {
		t.Fatalf("expected snapshot count 2, got %d", filefinder.SnapshotCount(snap))
	}

	// Adding more docs after snapshot doesn't affect it.
	idx.Add("foo/qux.go", 0)
	if filefinder.SnapshotCount(snap) != 2 {
		t.Fatal("snapshot count should not change after new adds")
	}
}

func TestIndex_TrigramIndex(t *testing.T) {
	t.Parallel()
	idx := filefinder.NewIndex()
	idx.Add("handler.go", 0)

	// "handler.go" should produce trigrams for "handler.go".
	// Check that at least one trigram exists.
	if filefinder.IndexByGramLen(idx) == 0 {
		t.Fatal("expected non-empty trigram index")
	}
}

func TestIndex_PrefixIndex(t *testing.T) {
	t.Parallel()
	idx := filefinder.NewIndex()
	idx.Add("handler.go", 0)

	// basename is "handler.go", first byte is 'h'
	if filefinder.IndexByPrefix1Len(idx, 'h') == 0 {
		t.Fatal("expected prefix1['h'] to be non-empty")
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
