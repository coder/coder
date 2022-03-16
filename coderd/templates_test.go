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
	require.Len(t, templates, 0)
}

func TestTemplateArchive(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	templates, err := client.Templates(context.Background())
	require.NoError(t, err)
	require.Len(t, templates, 0)
}
