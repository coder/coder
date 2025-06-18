package cli

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

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
