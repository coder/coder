package chatagent_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatagent"
)

func TestIsChatAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "ExactSuffix",
			input: "agent-coderd-chat",
			want:  true,
		},
		{
			name:  "UppercaseSuffix",
			input: "agent-CODERD-CHAT",
			want:  true,
		},
		{
			name:  "MixedCaseSuffix",
			input: "agent-Coderd-Chat",
			want:  true,
		},
		{
			name:  "NoSuffix",
			input: "my-agent",
			want:  false,
		},
		{
			name:  "SuffixOnly",
			input: "-coderd-chat",
			want:  true,
		},
		{
			name:  "PartialSuffix",
			input: "agent-coderd",
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, chatagent.IsChatAgent(tt.input))
		})
	}
}
