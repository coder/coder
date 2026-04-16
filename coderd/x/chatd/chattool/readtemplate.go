//go:build !slim

package chattool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// ReadTemplateOptions configures the read_template tool.
type ReadTemplateOptions struct {
	DB                 database.Store
	OrganizationID     uuid.UUID
	OwnerID            uuid.UUID
	AllowedTemplateIDs func() map[uuid.UUID]bool
}

type readTemplateArgs struct {
	TemplateID string `json:"template_id" description:"The UUIDv4 of the template to read details for. Obtain this from list_templates."`
}

// ReadTemplate returns a tool that retrieves details about a specific
// template, including its configurable rich parameters. The agent
// uses this after list_templates and before create_workspace.
func ReadTemplate(options ReadTemplateOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"read_template",
		"Get details about a workspace template, including its "+
			"configurable parameters and available presets. Use this "+
			"after finding a template with list_templates and before "+
			"creating a workspace with create_workspace.",
		func(ctx context.Context, args readTemplateArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.DB == nil {
				return fantasy.NewTextErrorResponse("database is not configured"), nil
			}

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

			var allowlist map[uuid.UUID]bool
			if options.AllowedTemplateIDs != nil {
				allowlist = options.AllowedTemplateIDs()
			}

			ctx, err = AsOwner(ctx, options.DB, options.OwnerID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			readResult, err := ReadTemplateHelper(ctx, options.DB, allowlist, templateID)
			if err != nil {
				if errors.Is(err, ErrTemplateNotFound) {
					return fantasy.NewTextErrorResponse("template not found"), nil
				}
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if options.OrganizationID != uuid.Nil && readResult.Template.OrganizationID != options.OrganizationID {
				return fantasy.NewTextErrorResponse("template not found"), nil
			}

			presets, err := options.DB.GetPresetsByTemplateVersionID(ctx, readResult.Template.ActiveVersionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("failed to get template presets: %w", err).Error(),
				), nil
			}

			templateInfo := map[string]any{
				"id":                readResult.Template.ID.String(),
				"name":              readResult.Template.Name,
				"active_version_id": readResult.Template.ActiveVersionID.String(),
			}
			if display := strings.TrimSpace(readResult.Template.DisplayName); display != "" {
				templateInfo["display_name"] = display
			}
			if desc := strings.TrimSpace(readResult.Template.Description); desc != "" {
				templateInfo["description"] = desc
			}

			paramList := make([]map[string]any, 0, len(readResult.Parameters))
			for _, paramDetail := range readResult.Parameters {
				param := map[string]any{
					"name":     paramDetail.Name,
					"type":     paramDetail.Type,
					"required": paramDetail.Required,
				}
				if display := strings.TrimSpace(paramDetail.DisplayName); display != "" {
					param["display_name"] = display
				}
				if desc := strings.TrimSpace(paramDetail.Description); desc != "" {
					param["description"] = truncateRunes(desc, 300)
				}
				if paramDetail.DefaultValue != "" {
					param["default"] = paramDetail.DefaultValue
				}
				if paramDetail.Mutable {
					param["mutable"] = true
				}
				if paramDetail.Ephemeral {
					param["ephemeral"] = true
				}
				if paramDetail.FormType != "" {
					param["form_type"] = paramDetail.FormType
				}
				if len(paramDetail.Options) > 0 && string(paramDetail.Options) != "null" && string(paramDetail.Options) != "[]" {
					var opts []map[string]any
					if err := json.Unmarshal(paramDetail.Options, &opts); err == nil && len(opts) > 0 {
						param["options"] = opts
					}
				}
				if paramDetail.ValidationRegex != "" {
					param["validation_regex"] = paramDetail.ValidationRegex
				}
				if paramDetail.ValidationMin.Valid {
					param["validation_min"] = paramDetail.ValidationMin.Int32
				}
				if paramDetail.ValidationMax.Valid {
					param["validation_max"] = paramDetail.ValidationMax.Int32
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
				presetParams, err := options.DB.GetPresetParametersByTemplateVersionID(ctx, readResult.Template.ActiveVersionID)
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
