package chattool

import (
	"context"
	"encoding/json"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// ReadTemplateOptions configures the read_template tool.
type ReadTemplateOptions struct {
	OwnerID            uuid.UUID
	AllowedTemplateIDs func() map[uuid.UUID]bool
}

type readTemplateArgs struct {
	TemplateID string `json:"template_id" description:"The UUIDv4 of the template to read details for. Obtain this from list_templates."`
}

// ReadTemplate returns a tool that retrieves details about a specific
// template, including its configurable rich parameters. The agent
// uses this after list_templates and before create_workspace.
// db must not be nil.
func ReadTemplate(db database.Store, organizationID uuid.UUID, options ReadTemplateOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"read_template",
		"Get details about a workspace template, including its "+
			"configurable parameters and available presets. Use this "+
			"after finding a template with list_templates and before "+
			"creating a workspace with create_workspace.",
		func(ctx context.Context, args readTemplateArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			templateIDStr := strings.TrimSpace(args.TemplateID)
			if templateIDStr == "" {
				return fantasy.NewTextErrorResponse("template_id is required"), nil
			}
			templateID, err := uuid.Parse(templateIDStr)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("invalid template_id: %w", err).Error(),
				), nil
			}

			if !isTemplateAllowed(options.AllowedTemplateIDs, templateID) {
				return fantasy.NewTextErrorResponse("template not found"), nil
			}

			ctx, err = asOwner(ctx, db, options.OwnerID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			template, err := db.GetTemplateByID(ctx, templateID)
			if err != nil {
				return fantasy.NewTextErrorResponse("template not found"), nil
			}

			if template.OrganizationID != organizationID {
				return fantasy.NewTextErrorResponse("template not found"), nil
			}

			params, err := db.GetTemplateVersionParameters(ctx, template.ActiveVersionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("failed to get template parameters: %w", err).Error(),
				), nil
			}

			presets, err := db.GetPresetsByTemplateVersionID(ctx, template.ActiveVersionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("failed to get template presets: %w", err).Error(),
				), nil
			}

			templateInfo := map[string]any{
				"id":                template.ID.String(),
				"name":              template.Name,
				"active_version_id": template.ActiveVersionID.String(),
			}
			if display := strings.TrimSpace(template.DisplayName); display != "" {
				templateInfo["display_name"] = display
			}
			if desc := strings.TrimSpace(template.Description); desc != "" {
				templateInfo["description"] = desc
			}

			paramList := make([]map[string]any, 0, len(params))
			for _, p := range params {
				param := map[string]any{
					"name":     p.Name,
					"type":     p.Type,
					"required": p.Required,
				}
				if display := strings.TrimSpace(p.DisplayName); display != "" {
					param["display_name"] = display
				}
				if desc := strings.TrimSpace(p.Description); desc != "" {
					param["description"] = truncateRunes(desc, 300)
				}
				if p.DefaultValue != "" {
					param["default"] = p.DefaultValue
				}
				if p.Mutable {
					param["mutable"] = true
				}
				if p.Ephemeral {
					param["ephemeral"] = true
				}
				if p.FormType != "" {
					param["form_type"] = string(p.FormType)
				}
				if len(p.Options) > 0 && string(p.Options) != "null" && string(p.Options) != "[]" {
					var opts []map[string]any
					if err := json.Unmarshal(p.Options, &opts); err == nil && len(opts) > 0 {
						param["options"] = opts
					}
				}
				if p.ValidationRegex != "" {
					param["validation_regex"] = p.ValidationRegex
				}
				if p.ValidationMin.Valid {
					param["validation_min"] = p.ValidationMin.Int32
				}
				if p.ValidationMax.Valid {
					param["validation_max"] = p.ValidationMax.Int32
				}

				paramList = append(paramList, param)
			}

			result := map[string]any{
				"template":   templateInfo,
				"parameters": paramList,
			}

			// Include presets only when the template has them
			// to avoid cluttering responses.
			if len(presets) > 0 {
				presetParams, err := db.GetPresetParametersByTemplateVersionID(ctx, template.ActiveVersionID)
				if err != nil {
					return fantasy.NewTextErrorResponse(
						xerrors.Errorf("failed to get preset parameters: %w", err).Error(),
					), nil
				}

				// Index preset parameters by preset ID for
				// efficient lookup.
				paramsByPreset := make(map[uuid.UUID][]map[string]any)
				for _, pp := range presetParams {
					paramsByPreset[pp.TemplateVersionPresetID] = append(
						paramsByPreset[pp.TemplateVersionPresetID],
						map[string]any{
							"name":  pp.Name,
							"value": pp.Value,
						},
					)
				}

				presetList := make([]map[string]any, 0, len(presets))
				for _, p := range presets {
					preset := map[string]any{
						"id":      p.ID.String(),
						"name":    p.Name,
						"default": p.IsDefault,
					}
					if desc := strings.TrimSpace(p.Description); desc != "" {
						preset["description"] = desc
					}
					if icon := strings.TrimSpace(p.Icon); icon != "" {
						preset["icon"] = icon
					}
					// Surface the prebuild count when set so the LLM can prefer
					// presets backed by prebuilt workspaces. Match the toolsdk
					// `desired_prebuild_instances` key for cross-surface consistency.
					if p.DesiredInstances.Valid && p.DesiredInstances.Int32 > 0 {
						preset["desired_prebuild_instances"] = p.DesiredInstances.Int32
					}
					if params, ok := paramsByPreset[p.ID]; ok {
						preset["parameters"] = params
					} else {
						preset["parameters"] = []map[string]any{}
					}
					presetList = append(presetList, preset)
				}
				result["presets"] = presetList
			}

			return toolResponse(result), nil
		},
	)
}
