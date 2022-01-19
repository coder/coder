package validate

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/pointer"
)

func Test_DevURLName(t *testing.T) {
	testCases := []struct {
		S   string
		Err *string
	}{
		{"", nil},
		{"a", nil},
		{"a1", nil},
		{"a1a", nil},
		{"a-b", nil},
		{"a_b", nil},
		{"a_-b", nil},
		{"a_bc", nil},
		{"a-b-c", nil},
		{"a-b_c", nil},
		{"a_b-c", nil},
		{"a_b_c", nil},
		{"1", pointer.String("names must begin with a letter")},
		{"1a", pointer.String("names must begin with a letter")},
		{"1a1", pointer.String("names must begin with a letter")},
		{"1234", pointer.String("names must begin with a letter")},
		{"-a", pointer.String("names must begin with a letter")},
		{"a-", pointer.String("names must begin with a letter")},
		{"_a", pointer.String("names must begin with a letter")},
		{"a_", pointer.String("names must begin with a letter")},
		{"a--b", pointer.String("names must begin with a letter")},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pointer.String("names may not be more than 64 characters in length")},
	}
	for _, tc := range testCases {
		err := DevURLName(tc.S)
		if tc.Err != nil {
			require.Errorf(t, err, "expected error for test case %q", tc.S)
			require.Containsf(t, err.Error(), *tc.Err, "expected error for test case %q", tc.S)
		} else {
			require.NoError(t, err, tc.S)
		}
	}
}
