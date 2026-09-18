package chattool

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// ReadTemplateReadmeMaxRunes bounds the full README returned by read_template
// so one large README cannot dominate a single tool response.
const ReadTemplateReadmeMaxRunes = 8000

const (
	readTemplateOwnerDefaultsNote = "Parameter defaults were evaluated for " +
		"the workspace owner. Omitting a parameter in create_workspace uses " +
		"the listed default, and the build matches a preset by the resulting " +
		"values; a preset marked default is not applied implicitly."
	readTemplateImportDefaultsNote = "Parameter defaults are the values " +
		"recorded at template import and may differ from what the workspace " +
		"owner receives. Pass parameters or a preset_id explicitly when the " +
		"value matters."
)

// RenderTemplateParametersFn evaluates a template version's parameters as the
// workspace owner would see them on the creation form, so defaults derived
// from owner attributes such as groups resolve to the values a build uses.
type RenderTemplateParametersFn func(
	ctx context.Context,
	ownerID uuid.UUID,
	templateVersionID uuid.UUID,
) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error)

// ReadTemplateOptions configures the read_template tool.
type ReadTemplateOptions struct {
	OwnerID uuid.UUID
	// RenderParameters resolves parameter defaults for the owner. When nil,
	// or when evaluation fails, the tool falls back to the defaults recorded
	// at template import and says so in the response.
	RenderParameters RenderTemplateParametersFn
	Logger           slog.Logger
}

type readTemplateArgs struct {
	TemplateID string `json:"template_id" description:"The UUIDv4 of the template to read details for. Obtain this from list_templates."`
}

// ReadTemplate returns a tool that retrieves details about a specific
// template, including its configurable rich parameters. The agent uses
// this after list_templates when it needs parameters or presets before
// create_workspace.
// db must not be nil.
func ReadTemplate(db database.Store, organizationID uuid.UUID, options ReadTemplateOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"read_template",
		"Get details about a workspace template, including its "+
			"configurable parameters, available presets, and the active "+
			"version README. Parameter defaults are evaluated for the "+
			"workspace owner, so they are the values create_workspace uses "+
			"when a parameter is omitted. Use this after list_templates when "+
			"you need parameter details, preset IDs, or the README before "+
			"create_workspace.",
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
			if !template.AgentsAllowed {
				return fantasy.NewTextErrorResponse(templateNotAvailableMessage), nil
			}

			paramList, paramNote := readTemplateParameters(ctx, db, template, options)
			if paramList == nil {
				return fantasy.NewTextErrorResponse(paramNote), nil
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
			// Best-effort: a missing or unreadable version must not fail
			// read_template.
			if version, err := db.GetTemplateVersionByID(ctx, template.ActiveVersionID); err == nil {
				if r := readmeText(version.Readme, ReadTemplateReadmeMaxRunes); r != "" {
					templateInfo["readme"] = r
				}
			}

			result := map[string]any{
				"template":        templateInfo,
				"parameters":      paramList,
				"parameters_note": paramNote,
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

// readTemplateParameters returns the parameter list for the active template
// version and a note describing how defaults were derived. It prefers
// owner-evaluated parameters and falls back to the import-time rows when no
// renderer is configured or evaluation fails. A nil list means the fallback
// itself failed and the note carries the error.
func readTemplateParameters(
	ctx context.Context,
	db database.Store,
	template database.Template,
	options ReadTemplateOptions,
) ([]map[string]any, string) {
	if options.RenderParameters != nil {
		start := time.Now()
		rendered, diags, err := options.RenderParameters(ctx, options.OwnerID, template.ActiveVersionID)
		fields := []slog.Field{
			slog.F("template_id", template.ID),
			slog.F("template_version_id", template.ActiveVersionID),
			slog.F("owner_id", options.OwnerID),
			slog.F("duration", time.Since(start)),
		}
		if err == nil && !hasErrorDiagnostic(diags) {
			options.Logger.Debug(ctx, "read_template evaluated parameters for owner", fields...)
			paramList := make([]map[string]any, 0, len(rendered))
			for _, p := range rendered {
				paramList = append(paramList, renderedParameterEntry(p))
			}
			return paramList, readTemplateOwnerDefaultsNote
		}
		if err != nil {
			fields = append(fields, slog.Error(err))
		}
		for _, d := range diags {
			if d.Severity == codersdk.DiagnosticSeverityError {
				fields = append(fields, slog.F("diagnostic", d.Summary+": "+d.Detail))
			}
		}
		options.Logger.Warn(ctx, "read_template failed to evaluate parameters for owner, using import defaults", fields...)
	}

	params, err := db.GetTemplateVersionParameters(ctx, template.ActiveVersionID)
	if err != nil {
		return nil, xerrors.Errorf("failed to get template parameters: %w", err).Error()
	}
	paramList := make([]map[string]any, 0, len(params))
	for _, p := range params {
		paramList = append(paramList, staticParameterEntry(p))
	}
	return paramList, readTemplateImportDefaultsNote
}

func hasErrorDiagnostic(diags []codersdk.FriendlyDiagnostic) bool {
	for _, d := range diags {
		if d.Severity == codersdk.DiagnosticSeverityError {
			return true
		}
	}
	return false
}

// renderedParameterEntry converts an owner-evaluated parameter into the same
// shape as staticParameterEntry so the model sees one schema either way.
func renderedParameterEntry(p codersdk.PreviewParameter) map[string]any {
	param := map[string]any{
		"name":     p.Name,
		"type":     string(p.Type),
		"required": p.Required,
		"mutable":  p.Mutable,
	}
	if display := strings.TrimSpace(p.DisplayName); display != "" {
		param["display_name"] = display
	}
	if desc := strings.TrimSpace(p.Description); desc != "" {
		param["description"] = truncateRunes(desc, 300)
	}
	if p.DefaultValue.Valid && p.DefaultValue.Value != "" {
		param["default"] = p.DefaultValue.Value
	}
	if p.Ephemeral {
		param["ephemeral"] = true
	}
	if p.FormType != "" {
		param["form_type"] = string(p.FormType)
	}
	if len(p.Options) > 0 {
		opts := make([]map[string]any, 0, len(p.Options))
		for _, o := range p.Options {
			opt := map[string]any{
				"name":  o.Name,
				"value": o.Value.Value,
			}
			if desc := strings.TrimSpace(o.Description); desc != "" {
				opt["description"] = desc
			}
			if icon := strings.TrimSpace(o.Icon); icon != "" {
				opt["icon"] = icon
			}
			opts = append(opts, opt)
		}
		param["options"] = opts
	}
	for _, v := range p.Validations {
		if v.Regex != nil && *v.Regex != "" {
			param["validation_regex"] = *v.Regex
		}
		if v.Min != nil {
			param["validation_min"] = *v.Min
		}
		if v.Max != nil {
			param["validation_max"] = *v.Max
		}
	}
	return param
}

// staticParameterEntry converts an import-time parameter row into the tool's
// parameter shape.
func staticParameterEntry(p database.TemplateVersionParameter) map[string]any {
	param := map[string]any{
		"name":     p.Name,
		"type":     p.Type,
		"required": p.Required,
		"mutable":  p.Mutable,
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
	return param
}
