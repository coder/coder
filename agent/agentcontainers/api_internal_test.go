package agentcontainers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/provisioner"
)

func TestSafeAgentName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		folderName string
		expected   string
	}{
		// Basic valid names
		{
			folderName: "simple",
			expected:   "simple",
		},
		{
			folderName: "with-hyphens",
			expected:   "with-hyphens",
		},
		{
			folderName: "123numbers",
			expected:   "123numbers",
		},
		{
			folderName: "mixed123",
			expected:   "mixed123",
		},

		// Names that need transformation
		{
			folderName: "With_Underscores",
			expected:   "with-underscores",
		},
		{
			folderName: "With Spaces",
			expected:   "with-spaces",
		},
		{
			folderName: "UPPERCASE",
			expected:   "uppercase",
		},
		{
			folderName: "Mixed_Case-Name",
			expected:   "mixed-case-name",
		},

		// Names with special characters that get replaced
		{
			folderName: "special@#$chars",
			expected:   "special-chars",
		},
		{
			folderName: "dots.and.more",
			expected:   "dots-and-more",
		},
		{
			folderName: "multiple___underscores",
			expected:   "multiple-underscores",
		},
		{
			folderName: "multiple---hyphens",
			expected:   "multiple-hyphens",
		},

		// Edge cases with leading/trailing special chars
		{
			folderName: "-leading-hyphen",
			expected:   "leading-hyphen",
		},
		{
			folderName: "trailing-hyphen-",
			expected:   "trailing-hyphen",
		},
		{
			folderName: "_leading_underscore",
			expected:   "leading-underscore",
		},
		{
			folderName: "trailing_underscore_",
			expected:   "trailing-underscore",
		},
		{
			folderName: "---multiple-leading",
			expected:   "multiple-leading",
		},
		{
			folderName: "trailing-multiple---",
			expected:   "trailing-multiple",
		},

		// Complex transformation cases
		{
			folderName: "___very---complex@@@name___",
			expected:   "very-complex-name",
		},
		{
			folderName: "my.project-folder_v2",
			expected:   "my-project-folder-v2",
		},

		// Empty and fallback cases - now correctly uses friendlyName fallback
		{
			folderName: "",
			expected:   "friendly-fallback",
		},
		{
			folderName: "---",
			expected:   "friendly-fallback",
		},
		{
			folderName: "___",
			expected:   "friendly-fallback",
		},
		{
			folderName: "@#$",
			expected:   "friendly-fallback",
		},

		// Additional edge cases
		{
			folderName: "a",
			expected:   "a",
		},
		{
			folderName: "1",
			expected:   "1",
		},
		{
			folderName: "a1b2c3",
			expected:   "a1b2c3",
		},
		{
			folderName: "CamelCase",
			expected:   "camelcase",
		},
		{
			folderName: "snake_case_name",
			expected:   "snake-case-name",
		},
		{
			folderName: "kebab-case-name",
			expected:   "kebab-case-name",
		},
		{
			folderName: "mix3d_C4s3-N4m3",
			expected:   "mix3d-c4s3-n4m3",
		},
		{
			folderName: "123-456-789",
			expected:   "123-456-789",
		},
		{
			folderName: "abc123def456",
			expected:   "abc123def456",
		},
		{
			folderName: "   spaces   everywhere   ",
			expected:   "spaces-everywhere",
		},
		{
			folderName: "unicode-café-naïve",
			expected:   "unicode-caf-na-ve",
		},
		{
			folderName: "path/with/slashes",
			expected:   "path-with-slashes",
		},
		{
			folderName: "file.tar.gz",
			expected:   "file-tar-gz",
		},
		{
			folderName: "version-1.2.3-alpha",
			expected:   "version-1-2-3-alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.folderName, func(t *testing.T) {
			t.Parallel()
			name := safeAgentName(tt.folderName, "friendly-fallback")

			assert.Equal(t, tt.expected, name)
			assert.True(t, provisioner.AgentNameRegex.Match([]byte(name)))
		})
	}
}
