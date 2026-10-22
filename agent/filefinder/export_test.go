package filefinder

import (
	"context"

	"github.com/spf13/afero"
)

// BuildTestIndex walks root and returns a populated Index, the same
// way Engine.AddRoot does but without starting a watcher.
func BuildTestIndex(root string) (*Index, error) {
	return BuildTestIndexWith(NewIgnorer(afero.NewOsFs(), ""), root)
}

// BuildTestIndexWith is BuildTestIndex with the given ignore rules.
func BuildTestIndexWith(ig *Ignorer, root string) (*Index, error) {
	idx, _, err := walkRoot(context.Background(), ig, root)
	return idx, err
}

// IgnorerUsesGit reports whether ig runs git rather than the fallback.
func IgnorerUsesGit(ig *Ignorer) bool {
	return ig.git != nil
}
