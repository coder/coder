package wsrelated_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/wsrelated"
)

type fieldsRoot struct {
	ID       string         `json:"id" wsrelated:""`
	Children []fieldsChild  `json:"children" wsrelated:"latest_build"`
	Hidden   string         `json:"hidden"`
	Skipped  string         `json:"-" wsrelated:""`
	Covered  *fieldsCovered `json:"covered,omitempty" wsrelated:"template"`
}

type fieldsChild struct {
	Name     string `json:"name" wsrelated:"latest_build"`
	Outdated bool   `json:"outdated" wsrelated:"template,latest_build"`
}

type fieldsCovered struct {
	Value  string                 `json:"value"`
	ByName map[string]fieldsValue `json:"by_name"`
}

type fieldsValue struct {
	Untagged int
}

func TestFields(t *testing.T) {
	t.Parallel()

	type summary struct {
		jsonPath  string
		tag       string
		inherited bool
	}
	var got []summary
	for _, f := range wsrelated.Fields(reflect.TypeFor[fieldsRoot]()) {
		got = append(got, summary{f.JSONPath, f.Tag, f.Inherited})
	}

	// Untagged and JSON-omitted fields of a tagged struct are skipped. Fields
	// of an untagged struct inherit the tag of the field that contains it.
	require.Equal(t, []summary{
		{"id", "", false},
		{"children", "latest_build", false},
		{"children[].name", "latest_build", false},
		{"children[].outdated", "template,latest_build", false},
		{"covered", "template", false},
		{"covered.value", "template", true},
		{"covered.by_name", "template", true},
		{"covered.by_name[].Untagged", "template", true},
	}, got)
}
