//go:build !slim
// +build !slim

package template_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/template"
)

func TestTemplate(t *testing.T) {
	t.Parallel()
	list := template.List()
	require.Greater(t, len(list), 0)

	_, exists := template.Archive(list[0].ID)
	require.True(t, exists)
}
