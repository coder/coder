//nolint:testpackage // Internal test for unexported ChatProvidersByFamilyPrecedence helper.
package database

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestChatProvidersByFamilyPrecedence(t *testing.T) {
	t.Parallel()

	provider := func(id, family string, createdAt time.Time) ChatProvider {
		return ChatProvider{
			ID:        uuid.MustParse(id),
			Provider:  family,
			CreatedAt: createdAt,
		}
	}

	timeAt := func(day int) time.Time {
		return time.Date(2024, time.January, day, 12, 0, 0, 0, time.UTC)
	}

	cases := []struct {
		name  string
		input []ChatProvider
		want  []ChatProvider
	}{
		{
			name:  "empty input",
			input: nil,
			want:  nil,
		},
		{
			name: "single row",
			input: []ChatProvider{
				provider("00000000-0000-0000-0000-000000000001", "openai", timeAt(2)),
			},
			want: []ChatProvider{
				provider("00000000-0000-0000-0000-000000000001", "openai", timeAt(2)),
			},
		},
		{
			name: "all same family keeps oldest",
			input: []ChatProvider{
				provider("00000000-0000-0000-0000-000000000003", "openai", timeAt(3)),
				provider("00000000-0000-0000-0000-000000000001", "openai", timeAt(1)),
				provider("00000000-0000-0000-0000-000000000002", "openai", timeAt(2)),
			},
			want: []ChatProvider{
				provider("00000000-0000-0000-0000-000000000001", "openai", timeAt(1)),
			},
		},
		{
			name: "multiple families keep oldest per family",
			input: []ChatProvider{
				provider("00000000-0000-0000-0000-000000000006", "openai", timeAt(4)),
				provider("00000000-0000-0000-0000-000000000004", "anthropic", timeAt(2)),
				provider("00000000-0000-0000-0000-000000000008", "zeta", timeAt(3)),
				provider("00000000-0000-0000-0000-000000000005", "openai", timeAt(1)),
				provider("00000000-0000-0000-0000-000000000007", "anthropic", timeAt(5)),
			},
			want: []ChatProvider{
				provider("00000000-0000-0000-0000-000000000004", "anthropic", timeAt(2)),
				provider("00000000-0000-0000-0000-000000000005", "openai", timeAt(1)),
				provider("00000000-0000-0000-0000-000000000008", "zeta", timeAt(3)),
			},
		},
		{
			name: "same provider and timestamp breaks ties by id",
			input: []ChatProvider{
				provider("00000000-0000-0000-0000-00000000000b", "openai", timeAt(1)),
				provider("00000000-0000-0000-0000-00000000000a", "openai", timeAt(1)),
			},
			want: []ChatProvider{
				provider("00000000-0000-0000-0000-00000000000a", "openai", timeAt(1)),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ChatProvidersByFamilyPrecedence(tc.input)

			require.Equal(t, tc.want, got)
		})
	}
}
