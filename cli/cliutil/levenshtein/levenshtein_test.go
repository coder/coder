package levenshtein_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/cliutil/levenshtein"
)

func Test_Levenshtein_Matches(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		Name        string
		Needle      string
		MaxDistance int
		Haystack    []string
		Expected    []string
	}{
		{
			Name:        "empty",
			Needle:      "",
			MaxDistance: 0,
			Haystack:    []string{},
			Expected:    []string{},
		},
		{
			Name:        "empty haystack",
			Needle:      "foo",
			MaxDistance: 0,
			Haystack:    []string{},
			Expected:    []string{},
		},
		{
			Name:        "empty needle",
			Needle:      "",
			MaxDistance: 0,
			Haystack:    []string{"foo"},
			Expected:    []string{},
		},
		{
			Name:        "exact match distance 0",
			Needle:      "foo",
			MaxDistance: 0,
			Haystack:    []string{"foo", "fob"},
			Expected:    []string{"foo"},
		},
		{
			Name:        "exact match distance 1",
			Needle:      "foo",
			MaxDistance: 1,
			Haystack:    []string{"foo", "bar"},
			Expected:    []string{"foo"},
		},
		{
			Name:        "not found",
			Needle:      "foo",
			MaxDistance: 1,
			Haystack:    []string{"bar"},
			Expected:    []string{},
		},
		{
			Name:        "1 deletion",
			Needle:      "foo",
			MaxDistance: 1,
			Haystack:    []string{"bar", "fo"},
			Expected:    []string{"fo"},
		},
		{
			Name:        "one deletion, two matches",
			Needle:      "foo",
			MaxDistance: 1,
			Haystack:    []string{"bar", "fo", "fou"},
			Expected:    []string{"fo", "fou"},
		},
		{
			Name:        "one deletion, one addition",
			Needle:      "foo",
			MaxDistance: 1,
			Haystack:    []string{"bar", "fo", "fou", "f"},
			Expected:    []string{"fo", "fou"},
		},
		{
			Name:        "distance 2",
			Needle:      "foo",
			MaxDistance: 2,
			Haystack:    []string{"bar", "boo", "boof"},
			Expected:    []string{"boo", "boof"},
		},
		{
			Name:        "longer input",
			Needle:      "kuberenetes",
			MaxDistance: 5,
			Haystack:    []string{"kubernetes", "kubeconfig", "kubectl", "kube"},
			Expected:    []string{"kubernetes"},
		},
	} {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			actual := levenshtein.Matches(tt.Needle, tt.MaxDistance, tt.Haystack...)
			require.ElementsMatch(t, tt.Expected, actual)
		})
	}
}

func Test_Levenshtein_Distance(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		Name     string
		A        string
		B        string
		MaxDist  int
		Expected int
		Error    string
	}{
		{
			Name:     "empty",
			A:        "",
			B:        "",
			MaxDist:  -1,
			Expected: 0,
		},
		{
			Name:     "a empty",
			A:        "",
			B:        "foo",
			MaxDist:  -1,
			Expected: 3,
		},
		{
			Name:     "b empty",
			A:        "foo",
			B:        "",
			MaxDist:  -1,
			Expected: 3,
		},
		{
			Name:     "a is b",
			A:        "foo",
			B:        "foo",
			MaxDist:  -1,
			Expected: 0,
		},
		{
			Name:     "one addition",
			A:        "foo",
			B:        "fooo",
			MaxDist:  -1,
			Expected: 1,
		},
		{
			Name:     "one deletion",
			A:        "fooo",
			B:        "foo",
			MaxDist:  -1,
			Expected: 1,
		},
		{
			Name:     "one substitution",
			A:        "foo",
			B:        "fou",
			MaxDist:  -1,
			Expected: 1,
		},
		{
			Name:     "different strings entirely",
			A:        "foo",
			B:        "bar",
			MaxDist:  -1,
			Expected: 3,
		},
		{
			Name:    "different strings, max distance 2",
			A:       "foo",
			B:       "bar",
			MaxDist: 2,
			Error:   levenshtein.ErrMaxDist.Error(),
		},
	} {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			actual, err := levenshtein.Distance(tt.A, tt.B, tt.MaxDist)
			if tt.Error == "" {
				require.NoError(t, err)
				require.Equal(t, tt.Expected, actual)
			} else {
				require.EqualError(t, err, tt.Error)
			}
		})
	}
}
