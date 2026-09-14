package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/aibridge/mcp"
)

func TestGetClientInfo(t *testing.T) {
	t.Parallel()

	info := mcp.GetClientInfo()

	assert.Equal(t, "coder/aibridge", info.Name)
	assert.NotEmpty(t, info.Version)
	// Version will either be a git revision, a semantic version, or a combination
	assert.NotEqual(t, "", info.Version)
}
