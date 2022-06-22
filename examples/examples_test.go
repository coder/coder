package examples_test

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/examples"
)

func TestTemplate(t *testing.T) {
	t.Parallel()
	list, err := examples.List()
	require.NoError(t, err)
	require.NotEmpty(t, list)
	for _, eg := range list {
		eg := eg
		t.Run(eg.ID, func(t *testing.T) {
			t.Parallel()
			assert.NotEmpty(t, eg.ID, "example ID should not be empty")
			assert.NotEmpty(t, eg.URL, "example URL should not be empty")
			assert.NotEmpty(t, eg.Name, "example name should not be empty")
			assert.NotEmpty(t, eg.Description, "example description should not be empty")
			assert.NotEmpty(t, eg.Markdown, "example markdown should not be empty")
			_, err := examples.Archive(eg.ID)
			assert.NoError(t, err, "error archiving example")
		})
	}
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
