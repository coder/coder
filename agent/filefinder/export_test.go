package filefinder

import (
	"context"

	"github.com/spf13/afero"
)

// BuildTestIndex walks root and returns a populated Index, the same
// way Engine.AddRoot does but without starting a watcher.
func BuildTestIndex(root string) (*Index, error) {
	idx, _, err := walkRoot(context.Background(), NewIgnorer(afero.NewOsFs(), ""), root)
	return idx, err
}
