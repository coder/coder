package taskname

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestGenerateFallback(t *testing.T) {
	t.Parallel()

	taskName := generateFallback()
	err := codersdk.NameValid(taskName.Name)
	require.NoErrorf(t, err, "expected fallback to be valid workspace name, instead found %s", taskName.Name)
	require.NotEmpty(t, taskName.DisplayName)
}

func TestGenerateFromPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		prompt              string
		expectError         bool
		expectedName        string
		expectedDisplayName string
	}{
		{
			name:        "EmptyPrompt",
			prompt:      "",
			expectError: true,
		},
		{
			name:        "OnlySpaces",
			prompt:      "     ",
			expectError: true,
		},
		{
			name:        "OnlySpecialCharacters",
			prompt:      "!@#$%^&*()",
			expectError: true,
		},
		{
			name:                "UppercasePrompt",
			prompt:              "BUILD MY APP",
			expectError:         false,
			expectedName:        "build-my-app",
			expectedDisplayName: "BUILD MY APP",
		},
		{
			name:                "PromptWithApostrophes",
			prompt:              "fix user's dashboard",
			expectError:         false,
			expectedName:        "fix-users-dashboard",
			expectedDisplayName: "Fix user's dashboard",
		},
		{
			name:                "LongPrompt",
			prompt:              strings.Repeat("a", 100),
			expectError:         false,
			expectedName:        strings.Repeat("a", 27),
			expectedDisplayName: "A" + strings.Repeat("a", 62) + "…",
		},
		{
			name:                "PromptWithMultipleSpaces",
			prompt:              "build    my    app",
			expectError:         false,
			expectedName:        "build-my-app",
			expectedDisplayName: "Build    my    app",
		},
		{
			name:                "PromptWithNewlines",
			prompt:              "build\nmy\napp",
			expectError:         false,
			expectedName:        "build-my-app",
			expectedDisplayName: "Build my app",
		},
		{
			name:                "TruncatesLongPromptAtWordBoundary",
			prompt:              "implement real-time notifications dashboard",
			expectError:         false,
			expectedName:        "implement-real-time",
			expectedDisplayName: "Implement real-time notifications dashboard",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			taskName, err := generateFromPrompt(tc.prompt)

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Validate task name
			require.Contains(t, taskName.Name, fmt.Sprintf("%s-", tc.expectedName))
			require.NoError(t, codersdk.NameValid(taskName.Name))

			// Validate task display name
			require.NotEmpty(t, taskName.DisplayName)
			require.Equal(t, tc.expectedDisplayName, taskName.DisplayName)
		})
	}
}

func TestGenerateFromAnthropic(t *testing.T) {
	t.Parallel()

	apiKey := getAnthropicAPIKeyFromEnv()
	if apiKey == "" {
		t.Skip("Skipping test as ANTHROPIC_API_KEY not set")
	}

	tests := []struct {
		name   string
		prompt string
	}{
		{
			name:   "SimplePrompt",
			prompt: "Create a finance planning app",
		},
		{
			name:   "TechnicalPrompt",
			prompt: "Debug authentication middleware for OAuth2",
		},
		{
			name:   "ShortPrompt",
			prompt: "Fix bug",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)

			taskName, err := generateFromAnthropic(ctx, tc.prompt, apiKey, getAnthropicModelFromEnv())
			require.NoError(t, err)

			// Validate task name
			require.NotEmpty(t, taskName.DisplayName)
			require.NoError(t, codersdk.NameValid(taskName.Name))

			// Validate display name
			require.NotEmpty(t, taskName.DisplayName)
		})
	}
}
