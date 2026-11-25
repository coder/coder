package chattool

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

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
