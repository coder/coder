package filefinder

import (
	"testing"
)

func TestIndex_AddOverwrite(t *testing.T) {
	t.Parallel()
	idx := NewIndex()
	idx.Add("foo/bar.go", uint16(FlagFile))
	idx.Add("foo/bar.go", uint16(FlagDir)) // overwrite
	if idx.Len() != 1 {
		t.Fatalf("expected 1 after overwrite, got %d", idx.Len())
	}
	// The old entry should be tombstoned.
	if !idx.deleted[0] {
		t.Fatal("expected old entry to be deleted")
	}
	if idx.deleted[1] {
		t.Fatal("expected new entry to be live")
	}
}

func TestIndex_Snapshot(t *testing.T) {
	t.Parallel()
	idx := NewIndex()
	idx.Add("foo/bar.go", 0)
	idx.Add("foo/baz.go", 0)

	snap := idx.Snapshot()
	if len(snap.docs) != 2 {
		t.Fatalf("expected snapshot count 2, got %d", len(snap.docs))
	}

	// Adding more docs after snapshot doesn't affect it.
	idx.Add("foo/qux.go", 0)
	if len(snap.docs) != 2 {
		t.Fatal("snapshot count should not change after new adds")
	}
}

func TestIndex_TrigramIndex(t *testing.T) {
	t.Parallel()
	idx := NewIndex()
	idx.Add("handler.go", 0)

	// "handler.go" should produce trigrams for "handler.go".
	// Check that at least one trigram exists.
	if len(idx.byGram) == 0 {
		t.Fatal("expected non-empty trigram index")
	}
}

func TestIndex_PrefixIndex(t *testing.T) {
	t.Parallel()
	idx := NewIndex()
	idx.Add("handler.go", 0)

	// basename is "handler.go", first byte is 'h'
	if len(idx.byPrefix1['h']) == 0 {
		t.Fatal("expected prefix1['h'] to be non-empty")
	}
}
