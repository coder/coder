package site

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"testing/fstest"
)

func TestGetMetadataPathValidation(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"ok": &fstest.MapFile{Data: []byte("abc")},
	}
	cache := newBinMetadataCache(http.FS(fsys), map[string]string{})

	invalid := []string{"", "../x", "a/b", "/abs", ".", ".."}
	for _, name := range invalid {
		_, err := cache.getMetadata(name)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected os.ErrNotExist for %q, got %v", name, err)
		}
	}

	md, err := cache.getMetadata("ok")
	if err != nil {
		t.Fatalf("unexpected error for valid file: %v", err)
	}
	if md.sizeBytes <= 0 {
		t.Fatalf("expected positive size for valid file, got %d", md.sizeBytes)
	}
	if md.sha1Hash == "" {
		t.Fatalf("expected sha1Hash to be populated for valid file")
	}
}
