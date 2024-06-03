package terraform_test

import (
	"encoding/json"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform"
)

type hasDiagnostic struct {
	Diagnostic *tfjson.Diagnostic `json:"diagnostic"`
}

func TestFormatDiagnostic(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input    string
		expected []string
	}{
		"Expression": {
			input: `{"@level":"error","@message":"Error: Unsupported attribute","@module":"terraform.ui","@timestamp":"2023-03-17T10:33:38.761493+01:00","diagnostic":{"severity":"error","summary":"Unsupported attribute","detail":"This object has no argument, nested block, or exported attribute named \"foobar\".","range":{"filename":"main.tf","start":{"line":230,"column":81,"byte":5648},"end":{"line":230,"column":88,"byte":5655}},"snippet":{"context":"resource \"docker_container\" \"workspace\"","code":"  name = \"coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.foobar)}\"","start_line":230,"highlight_start_offset":80,"highlight_end_offset":87,"values":[]}},"type":"diagnostic"}`,
			expected: []string{
				"on main.tf line 230, in resource \"docker_container\" \"workspace\":",
				"  230:   name = \"coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.foobar)}\"",
				"",
				"This object has no argument, nested block, or exported attribute named \"foobar\".",
			},
		},
		"DynamicValues": {
			input: `{"@level":"error","@message":"Error: Invalid value for variable","@module":"terraform.ui","@timestamp":"2023-03-17T12:25:37.864793+01:00","diagnostic":{"severity":"error","summary":"Invalid value for variable","detail":"Invalid Digital Ocean Project ID.\n\nThis was checked by the validation rule at main.tf:27,3-13.","range":{"filename":"main.tf","start":{"line":18,"column":1,"byte":277},"end":{"line":18,"column":31,"byte":307}},"snippet":{"context":null,"code":"variable \"step1_do_project_id\" {","start_line":18,"highlight_start_offset":0,"highlight_end_offset":30,"values":[{"traversal":"var.step1_do_project_id","statement":"is \"magic-project-id\""}]}},"type":"diagnostic"}`,
			expected: []string{
				"on main.tf line 18:",
				"  18: variable \"step1_do_project_id\" {",
				"    ├────────────────",
				"    │ var.step1_do_project_id is \"magic-project-id\"",
				"",
				"Invalid Digital Ocean Project ID.",
				"",
				"This was checked by the validation rule at main.tf:27,3-13.",
			},
		},
	}

	for name, tc := range tests {
		tc := tc

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var d hasDiagnostic
			err := json.Unmarshal([]byte(tc.input), &d)
			require.NoError(t, err)

			output := terraform.FormatDiagnostic(d.Diagnostic)
			require.Equal(t, tc.expected, strings.Split(output, "\n"))
		})
	}
}
