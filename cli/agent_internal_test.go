package cli

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfinedAgentArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "equals form",
			args: []string{"coder", "agent", "--confine=proxy", "--agent-token", "token"},
			want: []string{"coder", "agent", "--agent-token", "token", "--no-reap"},
		},
		{
			name: "separate form",
			args: []string{"coder", "agent", "--confine", "netns", "--agent-token", "token"},
			want: []string{"coder", "agent", "--agent-token", "token", "--no-reap"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, confinedAgentArgs(tt.args))
		})
	}
}

func Test_extractPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		urlString string
		want      int
		wantErr   bool
	}{
		{
			name:      "Empty",
			urlString: "",
			wantErr:   true,
		},
		{
			name:      "NoScheme",
			urlString: "localhost:6060",
			want:      6060,
		},
		{
			name:      "WithScheme",
			urlString: "http://localhost:6060",
			want:      6060,
		},
		{
			name:      "NoPort",
			urlString: "http://localhost",
			wantErr:   true,
		},
		{
			name:      "NoPortNoScheme",
			urlString: "localhost",
			wantErr:   true,
		},
		{
			name:      "OnlyPort",
			urlString: "6060",
			wantErr:   true,
		},
		{
			name:      "127.0.0.1",
			urlString: "127.0.0.1:2113",
			want:      2113,
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := extractPort(tt.urlString)
			if tt.wantErr {
				require.Error(t, err, fmt.Sprintf("extractPort(%v)", tt.urlString))
			} else {
				require.NoError(t, err, fmt.Sprintf("extractPort(%v)", tt.urlString))
				require.Equal(t, tt.want, got, fmt.Sprintf("extractPort(%v)", tt.urlString))
			}
		})
	}
}
