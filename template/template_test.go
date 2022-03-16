package template_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/template"
)

func TestTemplate(t *testing.T) {
	t.Parallel()
	list, err := template.List()
	require.NoError(t, err)
	require.Greater(t, len(list), 0)

	_, err = template.Archive(list[0].ID)
	require.NoError(t, err)
}
