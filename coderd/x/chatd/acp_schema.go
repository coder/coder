package chatd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/toolschema"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type acpTool struct {
	info fantasy.ToolInfo
	opts fantasy.ProviderOptions
	run  func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error)
}

func (t *acpTool) Info() fantasy.ToolInfo                          { return t.info }
func (t *acpTool) ProviderOptions() fantasy.ProviderOptions        { return t.opts }
func (t *acpTool) SetProviderOptions(opts fantasy.ProviderOptions) { t.opts = opts }
func (t *acpTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return t.run(ctx, call)
}

func (t *acpTool) ToolInputSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": t.info.Parameters, "required": t.info.Required}
}

func newACPTool[T any](name, description string, properties map[string]any, required []string, run func(context.Context, T, fantasy.ToolCall) (fantasy.ToolResponse, error)) fantasy.AgentTool {
	validationProperties := acpValidationProperties(properties)
	return &acpTool{
		info: fantasy.ToolInfo{Name: name, Description: description, Parameters: properties, Required: required, Parallel: true},
		run: func(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(call.Input), &fields); err != nil || fields == nil {
				return fantasy.NewTextErrorResponse("expected one JSON object"), nil
			}
			for _, key := range required {
				if _, ok := fields[key]; !ok {
					return fantasy.NewTextErrorResponse("missing required ACP argument: " + key), nil
				}
			}
			for key, value := range fields {
				if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					return fantasy.NewTextErrorResponse("ACP argument cannot be null: " + key), nil
				}
			}
			if err := toolschema.ValidateUnambiguous(validationProperties, []byte(call.Input)); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			var args T
			decoder := json.NewDecoder(bytes.NewBufferString(call.Input))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&args); err != nil {
				return fantasy.NewTextErrorResponse("invalid ACP tool arguments: " + err.Error()), nil
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				return fantasy.NewTextErrorResponse("expected one JSON object"), nil
			}
			return run(ctx, args, call)
		},
	}
}

// Expose properties behind anyOf to the key validator. Configuration keys
// remain case-sensitive data, validated against the selected harness later.
func acpValidationProperties(properties map[string]any) map[string]any {
	result := make(map[string]any, len(properties))
	for key, value := range properties {
		schema, ok := value.(map[string]any)
		if !ok {
			result[key] = value
			continue
		}
		children := map[string]any{}
		if nested, ok := schema["properties"].(map[string]any); ok {
			children = acpValidationProperties(nested)
		}
		if variants, ok := schema["anyOf"].([]any); ok {
			for _, variant := range variants {
				if object, ok := variant.(map[string]any); ok {
					if nested, ok := object["properties"].(map[string]any); ok {
						for child, childSchema := range acpValidationProperties(nested) {
							children[child] = childSchema
						}
					}
				}
			}
		}
		result[key] = map[string]any{"properties": children}
	}
	return result
}

func acpHarnessesFromResources(resources []database.ChatContextResource) []workspacesdk.ACPHarness {
	harnesses := []workspacesdk.ACPHarness{}
	for _, resource := range resources {
		if resource.BodyKind != database.WorkspaceAgentContextBodyKindAcpHarness || resource.Status != database.WorkspaceAgentContextResourceStatusOk {
			continue
		}
		var body agentproto.ACPHarnessBody
		if contextBodyUnmarshalOptions.Unmarshal(resource.Body, &body) != nil || body.Slug == "" {
			continue
		}
		h := workspacesdk.ACPHarness{Slug: body.Slug, DisplayName: body.DisplayName, LoadSession: body.LoadSession, ResumeSession: body.ResumeSession, Steering: body.Steering}
		for _, option := range body.ConfigOptions {
			o := workspacesdk.ACPConfigOption{ID: option.Id, Name: option.Name, Description: option.Description, CurrentValue: option.CurrentValue, Category: option.Category}
			for _, value := range option.Values {
				o.Values = append(o.Values, workspacesdk.ACPConfigValue{ID: value.Id, Name: value.Name, Description: value.Description, Group: value.Group, GroupName: value.GroupName})
			}
			h.ConfigOptions = append(h.ConfigOptions, o)
		}
		harnesses = append(harnesses, h)
	}
	return harnesses
}

func acpSpawnProperties(harnesses []workspacesdk.ACPHarness) map[string]any {
	variants := []any{}
	for _, h := range harnesses {
		config := map[string]any{}
		for _, option := range h.ConfigOptions {
			if option.ID == "" || len(option.Values) == 0 {
				continue
			}
			values := []string{}
			description := option.Name + ". " + option.Description
			for _, value := range option.Values {
				values = append(values, value.ID)
				description += " " + value.ID + ": " + value.Name + ". " + value.Description
			}
			description += " Default: " + option.CurrentValue + "."
			config[option.ID] = map[string]any{"type": "string", "enum": values, "description": strings.TrimSpace(description)}
		}
		variants = append(variants, map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"harness"},
			"properties": map[string]any{
				"harness": map[string]any{"type": "string", "enum": []string{h.Slug}, "description": h.DisplayName},
				"config":  map[string]any{"type": "object", "additionalProperties": false, "properties": config},
			},
		})
	}
	return map[string]any{
		"prompt":            map[string]any{"type": "string", "description": "The initial task for the subagent, including what completes it."},
		"working_directory": map[string]any{"type": "string", "description": "Existing absolute directory where the subagent runs."},
		"agent":             map[string]any{"description": "The harness and optional session configuration overrides.", "anyOf": variants},
	}
}

func validateACPConfig(harness workspacesdk.ACPHarness, config map[string]string) error {
	for key, value := range config {
		valid := false
		for _, option := range harness.ConfigOptions {
			if option.ID != key {
				continue
			}
			for _, choice := range option.Values {
				valid = valid || choice.ID == value
			}
		}
		if !valid {
			return xerrors.Errorf("unsupported ACP configuration %q=%q", key, value)
		}
	}
	return nil
}
