package examples_test

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/examples"
)

func TestTemplate(t *testing.T) {
	t.Parallel()
	list, err := examples.List()
	require.NoError(t, err)
	require.Greater(t, len(list), 0)

	_, err = examples.Archive(list[0].ID)
	require.NoError(t, err)
}

func TestSubdirs(t *testing.T) {
	t.Parallel()
	tarData, err := examples.Archive("docker-image-builds")
	require.NoError(t, err)

	tarReader := tar.NewReader(bytes.NewReader(tarData))
	entryPaths := make(map[byte][]string)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		entryPaths[header.Typeflag] = append(entryPaths[header.Typeflag], header.Name)
	}

	require.Subset(t, entryPaths[tar.TypeDir], []string{"./", "images/"})
	require.Subset(t, entryPaths[tar.TypeReg], []string{"README.md", "main.tf", "images/base.Dockerfile"})
}
