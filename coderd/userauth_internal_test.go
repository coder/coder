package coderd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseStringSliceClaim(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name    string
		GoClaim interface{}
		// JSON Claim allows testing the json -> go conversion
		// of some strings.
		JSONClaim     string
		ErrorExpected bool
		ExpectedSlice []string
	}{
		{
			Name:          "Nil",
			GoClaim:       nil,
			ExpectedSlice: []string{},
		},
		// Go Slices
		{
			Name:          "EmptySlice",
			GoClaim:       []string{},
			ExpectedSlice: []string{},
		},
		{
			Name:          "StringSlice",
			GoClaim:       []string{"a", "b", "c"},
			ExpectedSlice: []string{"a", "b", "c"},
		},
		{
			Name:          "InterfaceSlice",
			GoClaim:       []interface{}{"a", "b", "c"},
			ExpectedSlice: []string{"a", "b", "c"},
		},
		{
			Name:          "MixedSlice",
			GoClaim:       []interface{}{"a", string("b"), interface{}("c")},
			ExpectedSlice: []string{"a", "b", "c"},
		},
		{
			Name:          "StringSliceOneElement",
			GoClaim:       []string{"a"},
			ExpectedSlice: []string{"a"},
		},
		// Json Slices
		{
			Name:          "JSONEmptySlice",
			JSONClaim:     `[]`,
			ExpectedSlice: []string{},
		},
		{
			Name:          "JSONStringSlice",
			JSONClaim:     `["a", "b", "c"]`,
			ExpectedSlice: []string{"a", "b", "c"},
		},
		{
			Name:          "JSONStringSliceOneElement",
			JSONClaim:     `["a"]`,
			ExpectedSlice: []string{"a"},
		},
		// Go string
		{
			Name:          "String",
			GoClaim:       "a",
			ExpectedSlice: []string{"a"},
		},
		{
			Name:          "EmptyString",
			GoClaim:       "",
			ExpectedSlice: []string{},
		},
		{
			Name:          "Interface",
			GoClaim:       interface{}("a"),
			ExpectedSlice: []string{"a"},
		},
		// JSON string
		{
			Name:          "JSONString",
			JSONClaim:     `"a"`,
			ExpectedSlice: []string{"a"},
		},
		{
			Name:          "JSONEmptyString",
			JSONClaim:     `""`,
			ExpectedSlice: []string{},
		},
		// Go Errors
		{
			Name:          "IntegerInSlice",
			GoClaim:       []interface{}{"a", "b", 1},
			ErrorExpected: true,
		},
		// Json Errors
		{
			Name:          "JSONIntegerInSlice",
			JSONClaim:     `["a", "b", 1]`,
			ErrorExpected: true,
		},
		{
			Name:          "JSON_CSV",
			JSONClaim:     `"a,b,c"`,
			ErrorExpected: true,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			if len(c.JSONClaim) > 0 {
				require.Nil(t, c.GoClaim, "go claim should be nil if json set")
				err := json.Unmarshal([]byte(c.JSONClaim), &c.GoClaim)
				require.NoError(t, err, "unmarshal json claim")
			}

			found, err := parseStringSliceClaim(c.GoClaim)
			if c.ErrorExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, c.ExpectedSlice, found, "expected groups")
			}
		})
	}
}
