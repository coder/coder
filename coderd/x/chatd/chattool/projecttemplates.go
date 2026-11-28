package chattool

import (
	"context"
	"encoding/json"
	"maps"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

const projectTemplateNotAvailableMessage = "template not available in this chat's project; use list_templates to find the project's templates"

// ProjectTemplates is a chat project's template configuration as one user
// sees it. Attaching a template never grants access to it, so attached
// templates the user cannot read are counted and never identified.
type ProjectTemplates struct {
	// Limit restricts the project's chats to Templates. When false, the
	// chats can use every template their owner can use.
	Limit bool
	// Templates are the attached templates the user can read, oldest
	// attachment first. Deleted, deprecated, and agent-disabled templates
	// are left out.
	Templates []database.Template
	// HiddenCount is the number of attached templates the user cannot read.
	HiddenCount int
	// DefaultTemplateID is the template the project recommends to its
	// chats. It and the defaults below are set only while that template is
	// one of Templates.
	DefaultTemplateID uuid.NullUUID
	// DefaultPresetID is applied when a chat creates a workspace from the
	// default template without choosing a preset.
	DefaultPresetID uuid.NullUUID
	// DefaultParameters are applied beneath the parameter values a chat
	// passes when it creates a workspace from the default template.
	DefaultParameters map[string]string
}

// Template returns the attached template with the given ID, if the user can
// read it.
func (p ProjectTemplates) Template(templateID uuid.UUID) (database.Template, bool) {
	index := slices.IndexFunc(p.Templates, func(t database.Template) bool { return t.ID == templateID })
	if index < 0 {
		return database.Template{}, false
	}
	return p.Templates[index], true
}

// Allows reports whether a chat in the project may create or attach a
// workspace that uses the template.
func (p ProjectTemplates) Allows(templateID uuid.UUID) bool {
	if !p.Limit {
		return true
	}
	_, ok := p.Template(templateID)
	return ok
}

// LoadProjectTemplates reads one project's templates. See
// LoadProjectsTemplates.
func LoadProjectTemplates(ctx context.Context, db database.Store, project database.ChatProject) (ProjectTemplates, error) {
	byProject, err := LoadProjectsTemplates(ctx, db, []database.ChatProject{project})
	if err != nil {
		return ProjectTemplates{}, err
	}
	return byProject[project.ID], nil
}

// LoadProjectsTemplates reads the templates of the given projects as the
// user ctx is authorized as, keyed by project ID. It reads live on every
// call, so changes to a project's attachments or to the user's access to a
// template apply immediately.
func LoadProjectsTemplates(ctx context.Context, db database.Store, projects []database.ChatProject) (map[uuid.UUID]ProjectTemplates, error) {
	byProject := make(map[uuid.UUID]ProjectTemplates, len(projects))
	if len(projects) == 0 {
		return byProject, nil
	}
	projectIDs := make([]uuid.UUID, 0, len(projects))
	for _, project := range projects {
		projectIDs = append(projectIDs, project.ID)
	}
	rows, err := db.GetChatProjectTemplatesByProjectIDs(ctx, projectIDs)
	if err != nil {
		return nil, xerrors.Errorf("get chat project templates: %w", err)
	}

	readable := make(map[uuid.UUID]database.Template, len(rows))
	// An empty IDs filter matches every template, so the lookup is skipped
	// when nothing is attached.
	if len(rows) > 0 {
		templateIDs := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			templateIDs = append(templateIDs, row.ChatProjectTemplate.TemplateID)
		}
		templates, err := db.GetTemplatesWithFilter(ctx, database.GetTemplatesWithFilterParams{IDs: templateIDs})
		if err != nil {
			return nil, xerrors.Errorf("get attached templates: %w", err)
		}
		for _, template := range templates {
			readable[template.ID] = template
		}
	}
	for _, row := range rows {
		entry := byProject[row.ChatProjectTemplate.ProjectID]
		if template, ok := readable[row.ChatProjectTemplate.TemplateID]; ok {
			entry.Templates = append(entry.Templates, template)
		} else {
			entry.HiddenCount++
		}
		byProject[row.ChatProjectTemplate.ProjectID] = entry
	}

	for _, project := range projects {
		entry := byProject[project.ID]
		entry.Limit = project.LimitTemplates
		if _, ok := entry.Template(project.DefaultTemplateID.UUID); ok && project.DefaultTemplateID.Valid {
			entry.DefaultTemplateID = project.DefaultTemplateID
			entry.DefaultPresetID = project.DefaultTemplateVersionPresetID
			if len(project.DefaultParameters) > 0 {
				if err := json.Unmarshal(project.DefaultParameters, &entry.DefaultParameters); err != nil {
					return nil, xerrors.Errorf("decode default parameters of chat project %s: %w", project.ID, err)
				}
			}
		}
		byProject[project.ID] = entry
	}
	return byProject, nil
}

// loadChatProjectTemplates reads the templates of the chat's project on every
// call so edits apply to running chats. It returns nil when the chat is not
// in a project, or its owner can no longer read the project, which leaves the
// chat with every agent-allowed template. ctx must carry the chat owner's
// authorization.
func loadChatProjectTemplates(ctx context.Context, db database.Store, projectID uuid.NullUUID) (*ProjectTemplates, error) {
	if !projectID.Valid {
		return nil, nil //nolint:nilnil // A nil project leaves the chat unrestricted.
	}
	project, err := db.GetChatProjectByID(ctx, projectID.UUID)
	if httpapi.Is404Error(err) {
		// Deleting a project detaches its chats, and an owner who lost
		// access to the project also loses its limit.
		return nil, nil //nolint:nilnil // A nil project leaves the chat unrestricted.
	}
	if err != nil {
		return nil, xerrors.Errorf("get chat project: %w", err)
	}
	templates, err := LoadProjectTemplates(ctx, db, project)
	if err != nil {
		return nil, err
	}
	return &templates, nil
}

// isDefault reports whether the template is the project's default template.
func (p *ProjectTemplates) isDefault(templateID uuid.UUID) bool {
	return p != nil && p.DefaultTemplateID.Valid && p.DefaultTemplateID.UUID == templateID
}

// applyProjectDefaults fills in the project's default preset and parameter
// values that the model did not pass. A preset the model chooses replaces
// the defaults entirely. The default preset is skipped when it no longer
// belongs to the template's active version, or when it sets a parameter the
// model passed, since preset values override build values. Lookup failures
// are returned so the model can retry instead of getting a workspace built
// without the project's defaults.
func applyProjectDefaults(
	ctx context.Context,
	logger slog.Logger,
	db database.Store,
	project *ProjectTemplates,
	activeVersionID uuid.UUID,
	args createWorkspaceArgs,
	req *codersdk.CreateWorkspaceRequest,
) error {
	if req.TemplateVersionPresetID != uuid.Nil {
		return nil
	}
	if project.DefaultPresetID.Valid {
		presets, err := db.GetPresetsByTemplateVersionID(ctx, activeVersionID)
		if err != nil {
			return xerrors.Errorf("load presets for project default preset: %w", err)
		}
		if slices.ContainsFunc(presets, func(p database.TemplateVersionPreset) bool { return p.ID == project.DefaultPresetID.UUID }) {
			overridden, err := presetSetsParameter(ctx, db, activeVersionID, project.DefaultPresetID.UUID, args.Parameters)
			if err != nil {
				return err
			}
			if !overridden {
				req.TemplateVersionPresetID = project.DefaultPresetID.UUID
			}
		} else {
			logger.Warn(ctx, "project default preset is not in the template's active version",
				slog.F("preset_id", project.DefaultPresetID.UUID),
				slog.F("template_version_id", activeVersionID),
			)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(project.DefaultParameters)) {
		if _, ok := args.Parameters[name]; ok {
			continue
		}
		req.RichParameterValues = append(req.RichParameterValues, codersdk.WorkspaceBuildParameter{
			Name:  name,
			Value: project.DefaultParameters[name],
		})
	}
	return nil
}

// presetSetsParameter reports whether the preset sets any of the given
// parameters.
func presetSetsParameter(
	ctx context.Context,
	db database.Store,
	versionID, presetID uuid.UUID,
	parameters map[string]string,
) (bool, error) {
	if len(parameters) == 0 {
		return false, nil
	}
	presetParameters, err := db.GetPresetParametersByTemplateVersionID(ctx, versionID)
	if err != nil {
		return false, xerrors.Errorf("load project default preset parameters: %w", err)
	}
	return slices.ContainsFunc(presetParameters, func(p database.TemplateVersionPresetParameter) bool {
		_, ok := parameters[p.Name]
		return p.TemplateVersionPresetID == presetID && ok
	}), nil
}
