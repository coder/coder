package v2 //nolint:testpackage // Tests unexported release helpers.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		wantErr bool
		want    version
	}{
		{
			input: "v2.21.0",
			want:  version{major: 2, minor: 21, patch: 0, rc: -1, original: "v2.21.0"},
		},
		{
			input: "v2.21.0-rc.3",
			want:  version{major: 2, minor: 21, patch: 0, rc: 3, original: "v2.21.0-rc.3"},
		},
		{
			input: "2.21.0",
			want:  version{major: 2, minor: 21, patch: 0, rc: -1, original: "v2.21.0"},
		},
		{
			input: "v0.0.0",
			want:  version{major: 0, minor: 0, patch: 0, rc: -1, original: "v0.0.0"},
		},
		{
			input: "v1.2.3-rc.0",
			want:  version{major: 1, minor: 2, patch: 3, rc: 0, original: "v1.2.3-rc.0"},
		},
		{
			input:   "not-a-version",
			wantErr: true,
		},
		{
			input:   "",
			wantErr: true,
		},
		{
			input:   "v1.2",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, err := parseVersion(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want.major, got.major, "major")
			require.Equal(t, tt.want.minor, got.minor, "minor")
			require.Equal(t, tt.want.patch, got.patch, "patch")
			require.Equal(t, tt.want.rc, got.rc, "rc")
			require.Equal(t, tt.want.original, got.original, "original")
		})
	}
}

func Test_versionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v    version
		want string
	}{
		{version{major: 2, minor: 21, patch: 0, rc: -1}, "v2.21.0"},
		{version{major: 2, minor: 21, patch: 0, rc: 3}, "v2.21.0-rc.3"},
		{version{major: 1, minor: 0, patch: 5, rc: -1}, "v1.0.5"},
		{version{major: 1, minor: 0, patch: 0, rc: 0}, "v1.0.0-rc.0"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.v.String())
		})
	}
}

func Test_versionIsRC(t *testing.T) {
	t.Parallel()

	require.True(t, version{rc: 0}.IsRC())
	require.True(t, version{rc: 3}.IsRC())
	require.False(t, version{rc: -1}.IsRC())
}
