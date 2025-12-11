package dynamicparameters

import (
	"context"
	"database/sql"
	"io/fs"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"

	"github.com/hashicorp/hcl/v2"
)

// Renderer is able to execute and evaluate terraform with the given inputs.
// It may use the database to fetch additional state, such as a user's groups,
// roles, etc. Therefore, it requires an authenticated `ctx`.
//
// 'Close()' **must** be called once the renderer is no longer needed.
// Forgetting to do so will result in a memory leak.
type Renderer interface {
	Render(ctx context.Context, ownerID uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics)
	RenderWithoutCache(ctx context.Context, ownerID uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics)
	Close()
}

var ErrTemplateVersionNotReady = xerrors.New("template version job not finished")

// RenderCache is an interface for caching preview.Preview results.
type RenderCache interface {
	get(templateVersionID, ownerID uuid.UUID, parameters map[string]string) (*preview.Output, bool)
	put(templateVersionID, ownerID uuid.UUID, parameters map[string]string, output *preview.Output)
	Close()
}

// noopRenderCache is a no-op implementation of RenderCache that doesn't cache anything.
type noopRenderCache struct{}

func (noopRenderCache) get(uuid.UUID, uuid.UUID, map[string]string) (*preview.Output, bool) {
	return nil, false
}

func (noopRenderCache) put(uuid.UUID, uuid.UUID, map[string]string, *preview.Output) {
	// no-op
}

func (noopRenderCache) Close() {
	// no-op
}

// loader is used to load the necessary coder objects for rendering a template
// version's parameters. The output is a Renderer, which is the object that uses
// the cached objects to render the template version's parameters.
type loader struct {
	templateVersionID uuid.UUID

	// cache of objects
	templateVersion        *database.TemplateVersion
	job                    *database.ProvisionerJob
	terraformValues        *database.TemplateVersionTerraformValue
	templateVariableValues *[]database.TemplateVersionVariable

	// renderCache caches preview.Preview results
	renderCache RenderCache
}

// Prepare is the entrypoint for this package. It loads the necessary objects &
// files from the database and returns a Renderer that can be used to render the
// template version's parameters.
func Prepare(ctx context.Context, db database.Store, cache files.FileAcquirer, versionID uuid.UUID, options ...func(r *loader)) (Renderer, error) {
	l := &loader{
		templateVersionID: versionID,
		renderCache:       noopRenderCache{},
	}

	for _, opt := range options {
		opt(l)
	}

	return l.Renderer(ctx, db, cache)
}

func WithTemplateVariableValues(vals []database.TemplateVersionVariable) func(r *loader) {
	return func(r *loader) {
		r.templateVariableValues = &vals
	}
}

func WithTemplateVersion(tv database.TemplateVersion) func(r *loader) {
	return func(r *loader) {
		if tv.ID == r.templateVersionID {
			r.templateVersion = &tv
		}
	}
}

func WithProvisionerJob(job database.ProvisionerJob) func(r *loader) {
	return func(r *loader) {
		r.job = &job
	}
}

func WithTerraformValues(values database.TemplateVersionTerraformValue) func(r *loader) {
	return func(r *loader) {
		if values.TemplateVersionID == r.templateVersionID {
			r.terraformValues = &values
		}
	}
}

func WithRenderCache(cache RenderCache) func(r *loader) {
	return func(r *loader) {
		r.renderCache = cache
	}
}

func (r *loader) loadData(ctx context.Context, db database.Store) error {
	if r.templateVersion == nil {
		tv, err := db.GetTemplateVersionByID(ctx, r.templateVersionID)
		if err != nil {
			return xerrors.Errorf("template version: %w", err)
		}
		r.templateVersion = &tv
	}

	if r.job == nil {
		job, err := db.GetProvisionerJobByID(ctx, r.templateVersion.JobID)
		if err != nil {
			return xerrors.Errorf("provisioner job: %w", err)
		}
		r.job = &job
	}

	if !r.job.CompletedAt.Valid {
		return ErrTemplateVersionNotReady
	}

	if r.terraformValues == nil {
		values, err := db.GetTemplateVersionTerraformValues(ctx, r.templateVersion.ID)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("template version terraform values: %w", err)
		}

		if xerrors.Is(err, sql.ErrNoRows) {
			// If the row does not exist, return zero values.
			//
			// Older template versions (prior to dynamic parameters) will be missing
			// this row, and we can assume the 'ProvisionerdVersion' "" (unknown).
			values = database.TemplateVersionTerraformValue{
				TemplateVersionID:   r.templateVersionID,
				UpdatedAt:           time.Time{},
				CachedPlan:          nil,
				CachedModuleFiles:   uuid.NullUUID{},
				ProvisionerdVersion: "",
			}
		}

		r.terraformValues = &values
	}

	if r.templateVariableValues == nil {
		vals, err := db.GetTemplateVersionVariables(ctx, r.templateVersion.ID)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("template version variables: %w", err)
		}
		r.templateVariableValues = &vals
	}

	return nil
}

// Renderer returns a Renderer that can be used to render the template version's
// parameters. It automatically determines whether to use a static or dynamic
// renderer based on the template version's state.
//
// Static parameter rendering is required to support older template versions that
// do not have the database state to support dynamic parameters. A constant
// warning will be displayed for these template versions.
func (r *loader) Renderer(ctx context.Context, db database.Store, cache files.FileAcquirer) (Renderer, error) {
	err := r.loadData(ctx, db)
	if err != nil {
		return nil, xerrors.Errorf("load data: %w", err)
	}

	if !ProvisionerVersionSupportsDynamicParameters(r.terraformValues.ProvisionerdVersion) {
		return r.staticRender(ctx, db)
	}

	return r.dynamicRenderer(ctx, db, files.NewCacheCloser(cache))
}

// Renderer caches all the necessary files when rendering a template version's
// parameters. It must be closed after use to release the cached files.
func (r *loader) dynamicRenderer(ctx context.Context, db database.Store, cache *files.CacheCloser) (*dynamicRenderer, error) {
	closeFiles := true // If the function returns with no error, this will toggle to false.
	defer func() {
		if closeFiles {
			cache.Close()
		}
	}()

	tfVarValues, err := VariableValues(*r.templateVariableValues)
	if err != nil {
		return nil, xerrors.Errorf("parse variable values: %w", err)
	}

	// If they can read the template version, then they can read the file for
	// parameter loading purposes.
	//nolint:gocritic
	fileCtx := dbauthz.AsFileReader(ctx)

	var templateFS fs.FS

	templateFS, err = cache.Acquire(fileCtx, db, r.job.FileID)
	if err != nil {
		return nil, xerrors.Errorf("acquire template file: %w", err)
	}

	var moduleFilesFS *files.CloseFS
	if r.terraformValues.CachedModuleFiles.Valid {
		moduleFilesFS, err = cache.Acquire(fileCtx, db, r.terraformValues.CachedModuleFiles.UUID)
		if err != nil {
			return nil, xerrors.Errorf("acquire module files: %w", err)
		}
		templateFS = files.NewOverlayFS(templateFS, []files.Overlay{{Path: ".terraform/modules", FS: moduleFilesFS}})
	}

	closeFiles = false // Caller will have to call close
	return &dynamicRenderer{
		data:        r,
		templateFS:  templateFS,
		db:          db,
		ownerErrors: make(map[uuid.UUID]error),
		close:       cache.Close,
		tfvarValues: tfVarValues,
	}, nil
}

type dynamicRenderer struct {
	db         database.Store
	data       *loader
	templateFS fs.FS

	ownerErrors  map[uuid.UUID]error
	currentOwner *previewtypes.WorkspaceOwner
	tfvarValues  map[string]cty.Value

	once  sync.Once
	close func()
}

func (r *dynamicRenderer) Render(ctx context.Context, ownerID uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics) {
	return r.render(ctx, ownerID, values, true)
}

func (r *dynamicRenderer) RenderWithoutCache(ctx context.Context, ownerID uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics) {
	return r.render(ctx, ownerID, values, false)
}

func (r *dynamicRenderer) render(ctx context.Context, ownerID uuid.UUID, values map[string]string, useCache bool) (*preview.Output, hcl.Diagnostics) {
	// Check cache first if enabled
	if useCache {
		if cached, ok := r.data.renderCache.get(r.data.templateVersionID, ownerID, values); ok {
			return cached, nil
		}
	}

	// Always start with the cached error, if we have one.
	ownerErr := r.ownerErrors[ownerID]
	if ownerErr == nil {
		ownerErr = r.getWorkspaceOwnerData(ctx, ownerID)
	}

	if ownerErr != nil || r.currentOwner == nil {
		r.ownerErrors[ownerID] = ownerErr
		return nil, hcl.Diagnostics{
			{
				Severity: hcl.DiagError,
				Summary:  "Failed to fetch workspace owner",
				Detail:   "Please check your permissions or the user may not exist.",
				Extra: previewtypes.DiagnosticExtra{
					Code: "owner_not_found",
				},
			},
		}
	}

	input := preview.Input{
		PlanJSON:        r.data.terraformValues.CachedPlan,
		ParameterValues: values,
		Owner:           *r.currentOwner,
		TFVars:          r.tfvarValues,
		// Do not emit parser logs to coderd output logs.
		// TODO: Returning this logs in the output would benefit the caller.
		//  Unsure how large the logs can be, so for now we just discard them.
		Logger: slog.New(slog.DiscardHandler),
	}

	output, diags := preview.Preview(ctx, input, r.templateFS)

	// Store in cache if successful and caching is enabled
	if useCache && !diags.HasErrors() {
		r.data.renderCache.put(r.data.templateVersionID, ownerID, values, output)
	}

	return output, diags
}

func (r *dynamicRenderer) getWorkspaceOwnerData(ctx context.Context, ownerID uuid.UUID) error {
	if r.currentOwner != nil && r.currentOwner.ID == ownerID.String() {
		return nil // already fetched
	}

	owner, err := WorkspaceOwner(ctx, r.db, r.data.templateVersion.OrganizationID, ownerID)
	if err != nil {
		return err
	}

	r.currentOwner = owner
	return nil
}

func (r *dynamicRenderer) Close() {
	r.once.Do(r.close)
}

func ProvisionerVersionSupportsDynamicParameters(version string) bool {
	major, minor, err := apiversion.Parse(version)
	// If the api version is not valid or less than 1.6, we need to use the static parameters
	useStaticParams := err != nil || major < 1 || (major == 1 && minor < 6)
	return !useStaticParams
}

func WorkspaceOwner(ctx context.Context, db database.Store, org uuid.UUID, ownerID uuid.UUID) (*previewtypes.WorkspaceOwner, error) {
	user, err := db.GetUserByID(ctx, ownerID)
	if err != nil {
		// If the user failed to read, we also try to read the user from their
		// organization member. You only need to be able to read the organization member
		// to get the owner data.
		//
		// Only the terraform files can therefore leak more information than the
		// caller should have access to. All this info should be public assuming you can
		// read the user though.
		mem, err := database.ExpectOne(db.OrganizationMembers(ctx, database.OrganizationMembersParams{
			OrganizationID: org,
			UserID:         ownerID,
			IncludeSystem:  true,
			GithubUserID:   0,
		}))
		if err != nil {
			return nil, xerrors.Errorf("fetch user: %w", err)
		}

		// Org member fetched, so use the provisioner context to fetch the user.
		//nolint:gocritic // Has the correct permissions, and matches the provisioning flow.
		user, err = db.GetUserByID(dbauthz.AsProvisionerd(ctx), mem.OrganizationMember.UserID)
		if err != nil {
			return nil, xerrors.Errorf("fetch user: %w", err)
		}
	}

	// nolint:gocritic // This is kind of the wrong query to use here, but it
	// matches how the provisioner currently works. We should figure out
	// something that needs less escalation but has the correct behavior.
	row, err := db.GetAuthorizationUserRoles(dbauthz.AsProvisionerd(ctx), ownerID)
	if err != nil {
		return nil, xerrors.Errorf("user roles: %w", err)
	}
	roles, err := row.RoleNames()
	if err != nil {
		return nil, xerrors.Errorf("expand roles: %w", err)
	}
	ownerRoles := make([]previewtypes.WorkspaceOwnerRBACRole, 0, len(roles))
	for _, it := range roles {
		if it.OrganizationID != uuid.Nil && it.OrganizationID != org {
			continue
		}
		var orgID string
		if it.OrganizationID != uuid.Nil {
			orgID = it.OrganizationID.String()
		}
		ownerRoles = append(ownerRoles, previewtypes.WorkspaceOwnerRBACRole{
			Name:  it.Name,
			OrgID: orgID,
		})
	}

	// The correct public key has to be sent. This will not be leaked
	// unless the template leaks it.
	// nolint:gocritic
	key, err := db.GetGitSSHKey(dbauthz.AsProvisionerd(ctx), ownerID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("ssh key: %w", err)
	}

	// The groups need to be sent to preview. These groups are not exposed to the
	// user, unless the template does it through the parameters. Regardless, we need
	// the correct groups, and a user might not have read access.
	// nolint:gocritic
	groups, err := db.GetGroups(dbauthz.AsProvisionerd(ctx), database.GetGroupsParams{
		OrganizationID: org,
		HasMemberID:    ownerID,
	})
	if err != nil {
		return nil, xerrors.Errorf("groups: %w", err)
	}
	groupNames := make([]string, 0, len(groups))
	for _, it := range groups {
		groupNames = append(groupNames, it.Group.Name)
	}

	return &previewtypes.WorkspaceOwner{
		ID:           user.ID.String(),
		Name:         user.Username,
		FullName:     user.Name,
		Email:        user.Email,
		LoginType:    string(user.LoginType),
		RBACRoles:    ownerRoles,
		SSHPublicKey: key.PublicKey,
		Groups:       groupNames,
	}, nil
}
