package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
)

func TestListTemplates(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	templates, err := client.Templates(context.Background())
	require.NoError(t, err)
	require.Greater(t, len(templates), 0)
}

func TestTemplateArchive(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	templates, err := client.Templates(context.Background())
	require.NoError(t, err)
	data, _, err := client.TemplateArchive(context.Background(), templates[0].ID)
	require.NoError(t, err)
	require.Greater(t, len(data), 0)
}
