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
	DB      database.Store
	OwnerID uuid.UUID
}

type readTemplateArgs struct {
	TemplateID string `json:"template_id"`
}

// ReadTemplate returns a tool that retrieves details about a specific
// template, including its configurable rich parameters. The agent
// uses this after list_templates and before create_workspace.
func ReadTemplate(options ReadTemplateOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"read_template",
		"Get details about a workspace template, including its "+
			"configurable parameters. Use this after finding a "+
			"template with list_templates and before creating a "+
			"workspace with create_workspace.",
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

			ctx, err = asOwner(ctx, options.DB, options.OwnerID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			template, err := options.DB.GetTemplateByID(ctx, templateID)
			if err != nil {
				return fantasy.NewTextErrorResponse("template not found"), nil
			}

			params, err := options.DB.GetTemplateVersionParameters(ctx, template.ActiveVersionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					xerrors.Errorf("failed to get template parameters: %w", err).Error(),
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

			return toolResponse(map[string]any{
				"template":   templateInfo,
				"parameters": paramList,
			}), nil
		},
	)
}
