package chatd

import (
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type acpSpawnTool struct {
	fantasy.AgentTool
	parameters map[string]any
}

func (t acpSpawnTool) Info() fantasy.ToolInfo {
	info := t.AgentTool.Info()
	info.Parameters = t.parameters
	return info
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
		h := workspacesdk.ACPHarness{Slug: body.Slug, DisplayName: body.DisplayName}
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
			"type": "object", "additionalProperties": false, "required": []string{"harness_id"},
			"properties": map[string]any{
				"harness_id": map[string]any{"type": "string", "enum": []string{h.Slug}, "description": h.DisplayName},
				"config":     map[string]any{"type": "object", "additionalProperties": false, "properties": config},
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
