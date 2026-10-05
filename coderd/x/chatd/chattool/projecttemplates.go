package chattool

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

const projectTemplateNotAvailableMessage = "template not available in this chat's project; use list_templates to find the project's templates"

// projectTemplates restricts a project chat to the project's templates and
// carries the project's workspace defaults.
type projectTemplates struct {
	templateIDs       []uuid.UUID
	defaultTemplateID uuid.NullUUID
	defaultPresetID   uuid.NullUUID
	defaultParameters map[string]string
}

// loadProjectTemplates reads the project's templates on every call so edits
// apply to running chats. It returns nil when the chat is not in a project,
// which leaves the chat with every agent-allowed template. ctx must carry
// the chat owner's authorization.
func loadProjectTemplates(ctx context.Context, db database.Store, projectID uuid.NullUUID) (*projectTemplates, error) {
	if !projectID.Valid {
		return nil, nil //nolint:nilnil // A nil policy leaves the chat unrestricted.
	}
	project, err := db.GetChatProjectByID(ctx, projectID.UUID)
	if errors.Is(err, sql.ErrNoRows) {
		// Deleting a project detaches its chats.
		return nil, nil //nolint:nilnil // A nil policy leaves the chat unrestricted.
	}
	if err != nil {
		return nil, xerrors.Errorf("get chat project: %w", err)
	}
	p := &projectTemplates{
		templateIDs:       project.TemplateIDs,
		defaultTemplateID: project.DefaultTemplateID,
		defaultPresetID:   project.DefaultTemplateVersionPresetID,
	}
	if len(project.DefaultParameters) > 0 {
		if err := json.Unmarshal(project.DefaultParameters, &p.defaultParameters); err != nil {
			return nil, xerrors.Errorf("decode project default parameters: %w", err)
		}
	}
	return p, nil
}

// allows reports whether the chat may use the template.
func (p *projectTemplates) allows(templateID uuid.UUID) bool {
	return p == nil || slices.Contains(p.templateIDs, templateID)
}

// isDefault reports whether the template is the project's default template.
func (p *projectTemplates) isDefault(templateID uuid.UUID) bool {
	return p != nil && p.defaultTemplateID.Valid && p.defaultTemplateID.UUID == templateID
}

// applyProjectDefaults fills in the project's default preset and parameter
// values that the model did not pass. A default preset that no longer
// belongs to the template's active version is skipped so the build does
// not fail on it.
func applyProjectDefaults(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	project *projectTemplates,
	activeVersionID uuid.UUID,
	args createWorkspaceArgs,
	req *codersdk.CreateWorkspaceRequest,
) {
	if req.TemplateVersionPresetID == uuid.Nil && project.defaultPresetID.Valid {
		presets, err := db.GetPresetsByTemplateVersionID(ctx, activeVersionID)
		switch {
		case err != nil:
			logger.Warn(ctx, "failed to load presets for project default preset", slog.Error(err))
		case slices.ContainsFunc(presets, func(p database.TemplateVersionPreset) bool { return p.ID == project.defaultPresetID.UUID }):
			req.TemplateVersionPresetID = project.defaultPresetID.UUID
		default:
			logger.Warn(ctx, "project default preset is not in the template's active version",
				slog.F("preset_id", project.defaultPresetID.UUID),
				slog.F("template_version_id", activeVersionID),
			)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(project.defaultParameters)) {
		if _, ok := args.Parameters[name]; ok {
			continue
		}
		req.RichParameterValues = append(req.RichParameterValues, codersdk.WorkspaceBuildParameter{
			Name:  name,
			Value: project.defaultParameters[name],
		})
	}
}
