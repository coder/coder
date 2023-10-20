package cliui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderAgentVersion(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		agentVersion  string
		serverVersion string
		expected      string
	}{
		{
			name:          "OK",
			agentVersion:  "v1.2.3",
			serverVersion: "v1.2.3",
			expected:      "v1.2.3",
		},
		{
			name:          "Outdated",
			agentVersion:  "v1.2.3",
			serverVersion: "v1.2.4",
			expected:      "v1.2.3 (outdated)",
		},
		{
			name:          "AgentUnknown",
			agentVersion:  "",
			serverVersion: "v1.2.4",
			expected:      "(unknown)",
		},
		{
			name:          "ServerUnknown",
			agentVersion:  "v1.2.3",
			serverVersion: "",
			expected:      "v1.2.3",
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual := renderAgentVersion(testCase.agentVersion, testCase.serverVersion)
			assert.Equal(t, testCase.expected, (actual))
		})
	}
}
