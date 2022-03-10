package template_test

import (
	"testing"

	"github.com/coder/coder/template"
	"github.com/stretchr/testify/require"
)

func TestTemplate(t *testing.T) {
	t.Parallel()
	list := template.List()
	require.Greater(t, len(list), 0)

	_, exists := template.Archive(list[0].ID)
	require.True(t, exists)
}
