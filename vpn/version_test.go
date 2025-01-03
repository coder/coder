package vpn_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/vpn"
)

func TestRPCVersionLatest(t *testing.T) {
	t.Parallel()
	require.NoError(t, vpn.CurrentSupportedVersions.Validate())
}

func TestRPCVersionParseString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  vpn.RPCVersion
	}{
		{
			name:  "valid version",
			input: "1.0",
			want:  vpn.RPCVersion{Major: 1, Minor: 0},
		},
		{
			name:  "valid version with larger numbers",
			input: "12.34",
			want:  vpn.RPCVersion{Major: 12, Minor: 34},
		},
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "one part",
			input: "1",
		},
		{
			name:  "three parts",
			input: "1.0.0",
		},
		{
			name:  "major version is 0",
			input: "0.1",
		},
		{
			name:  "invalid major version",
			input: "a.1",
		},
		{
			name:  "invalid minor version",
			input: "1.a",
		},
	}

	// nolint:paralleltest
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := vpn.ParseRPCVersion(tc.input)
			if tc.want.Major == 0 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)

				require.Equal(t, tc.input, got.String())
			}
		})
	}
}

func TestRPCVersionIsCompatibleWith(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		v1       vpn.RPCVersion
		v2       vpn.RPCVersion
		want     vpn.RPCVersion
		wantBool bool
	}{
		{
			name: "same version",
			v1:   vpn.RPCVersion{Major: 1, Minor: 0},
			v2:   vpn.RPCVersion{Major: 1, Minor: 0},
			want: vpn.RPCVersion{Major: 1, Minor: 0},
		},
		{
			name: "compatible minor versions",
			v1:   vpn.RPCVersion{Major: 1, Minor: 2},
			v2:   vpn.RPCVersion{Major: 1, Minor: 3},
			want: vpn.RPCVersion{Major: 1, Minor: 2},
		},
		{
			name: "incompatible major versions",
			v1:   vpn.RPCVersion{Major: 1, Minor: 0},
			v2:   vpn.RPCVersion{Major: 2, Minor: 0},
			want: vpn.RPCVersion{},
		},
	}

	// nolint:paralleltest
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := tc.v1.IsCompatibleWith(tc.v2)
			if tc.want.Major == 0 {
				require.False(t, ok)
				return
			}
			require.True(t, ok)
			require.Equal(t, got, tc.want)
		})
	}
}

func TestRPCVersionListParseString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		input       string
		want        vpn.RPCVersionList
		errContains string
	}{
		{
			name:  "single version",
			input: "1.0",
			want: vpn.RPCVersionList{
				Versions: []vpn.RPCVersion{
					{Major: 1, Minor: 0},
				},
			},
		},
		{
			name:  "multiple versions",
			input: "1.1,2.3,3.2",
			want: vpn.RPCVersionList{
				Versions: []vpn.RPCVersion{
					{Major: 1, Minor: 1},
					{Major: 2, Minor: 3},
					{Major: 3, Minor: 2},
				},
			},
		},
		{
			name:        "invalid version",
			input:       "1.0,invalid",
			errContains: "invalid version list",
		},
		{
			name:        "empty string",
			input:       "",
			errContains: "invalid version list",
		},
		{
			name:        "duplicate versions",
			input:       "1.0,1.0",
			errContains: "duplicate major version",
		},
		{
			name:        "duplicate major versions",
			input:       "1.0,1.2",
			errContains: "duplicate major version",
		},
		{
			name:        "out of order versions",
			input:       "2.0,1.0",
			errContains: "versions are not sorted",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := vpn.ParseRPCVersionList(tc.input)
			if tc.errContains != "" {
				require.ErrorContains(t, err, tc.errContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.input, got.String())
		})
	}
}

func TestRPCVersionListValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		list        vpn.RPCVersionList
		errContains string
	}{
		{
			name: "valid list",
			list: vpn.RPCVersionList{
				Versions: []vpn.RPCVersion{
					{Major: 1, Minor: 1},
					{Major: 2, Minor: 3},
					{Major: 3, Minor: 2},
				},
			},
		},
		{
			name: "empty list",
			list: vpn.RPCVersionList{
				Versions: []vpn.RPCVersion{},
			},
			errContains: "no versions",
		},
		{
			name: "duplicate versions",
			list: vpn.RPCVersionList{
				Versions: []vpn.RPCVersion{
					{Major: 1, Minor: 0},
					{Major: 1, Minor: 0},
				},
			},
			errContains: "duplicate major version",
		},
		{
			name: "duplicate major versions",
			list: vpn.RPCVersionList{
				Versions: []vpn.RPCVersion{
					{Major: 1, Minor: 0},
					{Major: 1, Minor: 2},
				},
			},
			errContains: "duplicate major version",
		},
		{
			name: "out of order versions",
			list: vpn.RPCVersionList{
				Versions: []vpn.RPCVersion{
					{Major: 2, Minor: 0},
					{Major: 1, Minor: 0},
				},
			},
			errContains: "versions are not sorted",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.list.Validate()
			if tc.errContains != "" {
				require.ErrorContains(t, err, tc.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
