package toolschema_test

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/toolschema"
)

func TestValidateUnambiguous(t *testing.T) {
	t.Parallel()

	readFile := chattool.ReadFile(chattool.ReadFileOptions{})
	editFiles := chattool.EditFiles(chattool.EditFilesOptions{})
	createWorkspace := chattool.CreateWorkspace(nil, uuid.Nil, uuid.Nil, chattool.CreateWorkspaceOptions{})

	tests := []struct {
		name    string
		tool    fantasy.AgentTool
		input   string
		wantErr string
	}{
		{
			name:    "case variant beside the canonical key",
			tool:    readFile,
			input:   `{"path":"/allowed","PATH":"/secret"}`,
			wantErr: `input key "PATH" differs from schema property "path" only by case`,
		},
		{
			name:    "case variant alone",
			tool:    readFile,
			input:   `{"PATH":"/secret"}`,
			wantErr: `input key "PATH" differs from schema property "path" only by case`,
		},
		{
			name:    "repeated key",
			tool:    readFile,
			input:   `{"path":"/allowed","path":"/secret"}`,
			wantErr: `input repeats the key "path"`,
		},
		{
			name:    "case variant nested in an array element",
			tool:    editFiles,
			input:   `{"files":[{"path":"a","PATH":"b","edits":[{"old_text":"x","new_text":"y"}]}]}`,
			wantErr: `input key "files[].PATH" differs from schema property "path" only by case`,
		},
		{
			name:    "case variant nested in an array element object",
			tool:    editFiles,
			input:   `{"files":[{"path":"a","edits":[{"old_text":"x","NEW_TEXT":"y"}]}]}`,
			wantErr: `input key "files[].edits[].NEW_TEXT" differs from schema property "new_text" only by case`,
		},
		{
			name:  "free-form map keys differing by case",
			tool:  createWorkspace,
			input: `{"template_id":"t","parameters":{"foo":"1","FOO":"2"}}`,
		},
		{
			name:    "free-form map repeating a key",
			tool:    createWorkspace,
			input:   `{"parameters":{"foo":"1","foo":"2"}}`,
			wantErr: `input repeats the key "parameters.foo"`,
		},
		{
			name:    "case variant inside a free-form map value",
			tool:    freeFormValueTool(),
			input:   `{"targets":{"first":{"PATH":"/secret"}}}`,
			wantErr: `input key "targets.first.PATH" differs from schema property "path" only by case`,
		},
		{
			name:  "key matching no property",
			tool:  readFile,
			input: `{"path":"/allowed","xyzzy":"b"}`,
		},
		{
			name:  "exact keys",
			tool:  editFiles,
			input: `{"files":[{"path":"a","edits":[{"old_text":"x","new_text":"y","replace_all":true}]}]}`,
		},
		{
			name:  "input the tool cannot decode either",
			tool:  readFile,
			input: `{"path":`,
		},
		{
			name:  "empty input",
			tool:  readFile,
			input: ``,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := toolschema.ValidateUnambiguous(tt.tool.Info().Parameters, []byte(tt.input))
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// TestCaseVariantKeyDecodesCaseInsensitively pins the decoder behavior the
// validator exists for: a case-sensitive reader of these bytes finds no
// "path" key in the first case and "/allowed" in the second, while the
// tool's own arguments resolve to "/secret" in both.
func TestCaseVariantKeyDecodesCaseInsensitively(t *testing.T) {
	t.Parallel()

	var args chattool.ReadFileArgs
	require.NoError(t, json.Unmarshal([]byte(`{"PATH":"/secret"}`), &args))
	require.Equal(t, "/secret", args.Path)

	require.NoError(t, json.Unmarshal([]byte(`{"path":"/allowed","PATH":"/secret"}`), &args))
	require.Equal(t, "/secret", args.Path)
}

func TestValidateUnambiguousComposedSchemas(t *testing.T) {
	t.Parallel()

	for _, keyword := range []string{"anyOf", "oneOf", "allOf"} {
		t.Run(keyword, func(t *testing.T) {
			t.Parallel()

			pathObject := map[string]any{"properties": map[string]any{"path": map[string]any{"type": "string"}}}
			sessionObject := map[string]any{"properties": map[string]any{"session_id": map[string]any{"type": "string"}}}
			properties := map[string]any{
				"target": map[string]any{
					"properties": map[string]any{"common": pathObject},
					keyword: []any{
						map[string]any{"type": "null"},
						map[string]any{"properties": map[string]any{
							"path":    map[string]any{"type": "string"},
							"label":   map[string]any{"type": "string"},
							"options": pathObject,
							"targets": map[string]any{"properties": map[string]any{"*": pathObject}},
						}},
						map[string]any{keyword: []any{
							map[string]any{"properties": map[string]any{
								"session_id": map[string]any{"type": "string"},
								"LABEL":      map[string]any{"type": "string"},
								"options":    sessionObject,
							}},
							map[string]any{"type": "array", "items": map[string]any{keyword: []any{pathObject, sessionObject}}},
						}},
					},
				},
			}
			for _, tt := range []struct {
				name    string
				input   string
				wantErr string
			}{
				{
					name:    "first branch",
					input:   `{"target":{"PATH":"secret"}}`,
					wantErr: `input key "target.PATH" differs from schema property "path" only by case`,
				},
				{
					name:    "nested branch",
					input:   `{"target":{"SESSION_ID":"secret"}}`,
					wantErr: `input key "target.SESSION_ID" differs from schema property "session_id" only by case`,
				},
				{
					name:    "properties beside branches",
					input:   `{"target":{"common":{"PATH":"secret"}}}`,
					wantErr: `input key "target.common.PATH" differs from schema property "path" only by case`,
				},
				{
					name:    "shared property first branch",
					input:   `{"target":{"options":{"PATH":"secret"}}}`,
					wantErr: `input key "target.options.PATH" differs from schema property "path" only by case`,
				},
				{
					name:    "shared property second branch",
					input:   `{"target":{"options":{"SESSION_ID":"secret"}}}`,
					wantErr: `input key "target.options.SESSION_ID" differs from schema property "session_id" only by case`,
				},
				{
					name:    "array first branch",
					input:   `{"target":[{"PATH":"secret"}]}`,
					wantErr: `input key "target[].PATH" differs from schema property "path" only by case`,
				},
				{
					name:    "array second branch",
					input:   `{"target":[{"SESSION_ID":"secret"}]}`,
					wantErr: `input key "target[].SESSION_ID" differs from schema property "session_id" only by case`,
				},
				{
					name:    "free-form map value",
					input:   `{"target":{"targets":{"first":{"PATH":"secret"}}}}`,
					wantErr: `input key "target.targets.first.PATH" differs from schema property "path" only by case`,
				},
				{
					name:    "duplicate key",
					input:   `{"target":{"options":{"path":"a","path":"b"}}}`,
					wantErr: `input repeats the key "target.options.path"`,
				},
				{
					name:    "exact key collides with nested branch",
					input:   `{"target":{"label":"a"}}`,
					wantErr: `input key "target.label" differs from schema property "LABEL" only by case`,
				},
				{
					name:    "exact key collides with first branch",
					input:   `{"target":{"LABEL":"b"}}`,
					wantErr: `input key "target.LABEL" differs from schema property "label" only by case`,
				},
				{
					name:    "case variants across branches",
					input:   `{"target":{"label":"a","LABEL":"b"}}`,
					wantErr: `input key "target.label" differs from schema property "LABEL" only by case`,
				},
				{
					name:    "case variants across branches reversed",
					input:   `{"target":{"LABEL":"b","label":"a"}}`,
					wantErr: `input key "target.LABEL" differs from schema property "label" only by case`,
				},
				{name: "exact keys across branches", input: `{"target":{"path":"a","session_id":"b","options":{"path":"a","session_id":"b"}}}`},
				{name: "unknown key", input: `{"target":{"other":"value"}}`},
				{name: "free-form keys", input: `{"target":{"targets":{"first":{"path":"a"},"FIRST":{"path":"b"}}}}`},
				{name: "null branch", input: `{"target":null}`},
			} {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					err := toolschema.ValidateUnambiguous(properties, []byte(tt.input))
					if tt.wantErr == "" {
						require.NoError(t, err)
						return
					}
					require.EqualError(t, err, tt.wantErr)
				})
			}
		})
	}
}

// freeFormValueTool builds a tool whose input nests a fixed property set
// inside a free-form map, so the value schema fantasy renders under "*" has
// to be carried into the map's values.
func freeFormValueTool() fantasy.AgentTool {
	type target struct {
		Path string `json:"path"`
	}
	type input struct {
		Targets map[string]target `json:"targets"`
	}
	return fantasy.NewAgentTool("free_form_value", "",
		func(context.Context, input, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.ToolResponse{}, nil
		})
}
