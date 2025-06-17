package dynamicparameters

import (
	"context"
	"encoding/json"
	"io/fs"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"

	"github.com/hashicorp/hcl/v2"
)

type Renderer interface {
	Render(ctx context.Context, ownerID uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics)
	Close()
}

var (
	ErrorTemplateVersionNotReady = xerrors.New("template version job not finished")
)

// Loader is used to load the necessary coder objects for rendering a template
// version's parameters. The output is a Renderer, which is the object that uses
// the cached objects to render the template version's parameters. Closing the
// Renderer will release the cached files.
type Loader struct {
	templateVersionID uuid.UUID

	// cache of objects
	templateVersion *database.TemplateVersion
	job             *database.ProvisionerJob
	terraformValues *database.TemplateVersionTerraformValue
}

func New(versionID uuid.UUID) *Loader {
	return &Loader{
		templateVersionID: versionID,
	}
}

func (r *Loader) WithTemplateVersion(tv database.TemplateVersion) *Loader {
	if tv.ID == r.templateVersionID {
		r.templateVersion = &tv
	}

	return r
}

func (r *Loader) WithProvisionerJob(job database.ProvisionerJob) *Loader {
	r.job = &job

	return r
}

func (r *Loader) WithTerraformValues(values database.TemplateVersionTerraformValue) *Loader {
	if values.TemplateVersionID == r.templateVersionID {
		r.terraformValues = &values
	}

	return r
}

func (r *Loader) Load(ctx context.Context, db database.Store) error {
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
		return ErrorTemplateVersionNotReady
	}

	if r.terraformValues == nil {
		values, err := db.GetTemplateVersionTerraformValues(ctx, r.templateVersion.ID)
		if err != nil {
			return xerrors.Errorf("template version terraform values: %w", err)
		}
		r.terraformValues = &values
	}

	return nil
}

func (r *Loader) loaded() bool {
	return r.templateVersion != nil && r.job != nil && r.terraformValues != nil
}

func (r *Loader) Renderer(ctx context.Context, db database.Store, cache *files.Cache) (Renderer, error) {
	if !r.loaded() {
		return nil, xerrors.New("Load() must be called before Renderer()")
	}

	if !ProvisionerVersionSupportsDynamicParameters(r.terraformValues.ProvisionerdVersion) {
		return r.staticRender(ctx, db)
	}

	return r.dynamicRenderer(ctx, db, cache)
}

// Renderer caches all the necessary files when rendering a template version's
// parameters. It must be closed after use to release the cached files.
func (r *Loader) dynamicRenderer(ctx context.Context, db database.Store, cache *files.Cache) (*DynamicRenderer, error) {
	// If they can read the template version, then they can read the file.
	fileCtx := dbauthz.AsFileReader(ctx)
	templateFS, err := cache.Acquire(fileCtx, r.job.FileID)
	if err != nil {
		return nil, xerrors.Errorf("acquire template file: %w", err)
	}

	var moduleFilesFS fs.FS
	if r.terraformValues.CachedModuleFiles.Valid {
		moduleFilesFS, err = cache.Acquire(fileCtx, r.terraformValues.CachedModuleFiles.UUID)
		if err != nil {
			cache.Release(r.job.FileID)
			return nil, xerrors.Errorf("acquire module files: %w", err)
		}
		templateFS = files.NewOverlayFS(templateFS, []files.Overlay{{Path: ".terraform/modules", FS: moduleFilesFS}})
	}

	plan := json.RawMessage("{}")
	if len(r.terraformValues.CachedPlan) > 0 {
		plan = r.terraformValues.CachedPlan
	}

	return &DynamicRenderer{
		data:         r,
		templateFS:   templateFS,
		db:           db,
		plan:         plan,
		failedOwners: make(map[uuid.UUID]error),
		close: func() {
			cache.Release(r.job.FileID)
			if moduleFilesFS != nil {
				cache.Release(r.terraformValues.CachedModuleFiles.UUID)
			}
		},
	}, nil
}

type DynamicRenderer struct {
	db         database.Store
	data       *Loader
	templateFS fs.FS
	plan       json.RawMessage

	failedOwners map[uuid.UUID]error
	currentOwner *previewtypes.WorkspaceOwner

	once  sync.Once
	close func()
}

func (r *DynamicRenderer) Render(ctx context.Context, ownerID uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics) {
	// Always start with the cached error, if we have one.
	ownerErr := r.failedOwners[ownerID]
	if ownerErr == nil {
		ownerErr = r.getWorkspaceOwnerData(ctx, ownerID)
	}

	if ownerErr != nil || r.currentOwner == nil {
		r.failedOwners[ownerID] = ownerErr
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
	}

	return preview.Preview(ctx, input, r.templateFS)
}

func (r *DynamicRenderer) getWorkspaceOwnerData(ctx context.Context, ownerID uuid.UUID) error {
	if r.currentOwner != nil && r.currentOwner.ID == ownerID.String() {
		return nil // already fetched
	}

	var g errgroup.Group

	// You only need to be able to read the organization member to get the owner
	// data. Only the terraform files can therefore leak more information than the
	// caller should have access to. All this info should be public assuming you can
	// read the user though.
	mem, err := database.ExpectOne(r.db.OrganizationMembers(ctx, database.OrganizationMembersParams{
		OrganizationID: r.data.templateVersion.OrganizationID,
		UserID:         ownerID,
		IncludeSystem:  false,
	}))
	if err != nil {
		return err
	}

	// User data is required for the form. Org member is checked above
	// nolint:gocritic
	user, err := r.db.GetUserByID(dbauthz.AsProvisionerd(ctx), mem.OrganizationMember.UserID)
	if err != nil {
		return xerrors.Errorf("fetch user: %w", err)
	}

	var ownerRoles []previewtypes.WorkspaceOwnerRBACRole
	g.Go(func() error {
		// nolint:gocritic // This is kind of the wrong query to use here, but it
		// matches how the provisioner currently works. We should figure out
		// something that needs less escalation but has the correct behavior.
		row, err := r.db.GetAuthorizationUserRoles(dbauthz.AsProvisionerd(ctx), ownerID)
		if err != nil {
			return err
		}
		roles, err := row.RoleNames()
		if err != nil {
			return err
		}
		ownerRoles = make([]previewtypes.WorkspaceOwnerRBACRole, 0, len(roles))
		for _, it := range roles {
			if it.OrganizationID != uuid.Nil && it.OrganizationID != r.data.templateVersion.OrganizationID {
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
		return nil
	})

	var publicKey string
	g.Go(func() error {
		// The correct public key has to be sent. This will not be leaked
		// unless the template leaks it.
		// nolint:gocritic
		key, err := r.db.GetGitSSHKey(dbauthz.AsProvisionerd(ctx), ownerID)
		if err != nil {
			return err
		}
		publicKey = key.PublicKey
		return nil
	})

	var groupNames []string
	g.Go(func() error {
		// The groups need to be sent to preview. These groups are not exposed to the
		// user, unless the template does it through the parameters. Regardless, we need
		// the correct groups, and a user might not have read access.
		// nolint:gocritic
		groups, err := r.db.GetGroups(dbauthz.AsProvisionerd(ctx), database.GetGroupsParams{
			OrganizationID: r.data.templateVersion.OrganizationID,
			HasMemberID:    ownerID,
		})
		if err != nil {
			return err
		}
		groupNames = make([]string, 0, len(groups))
		for _, it := range groups {
			groupNames = append(groupNames, it.Group.Name)
		}
		return nil
	})

	err = g.Wait()
	if err != nil {
		return err
	}

	r.currentOwner = &previewtypes.WorkspaceOwner{
		ID:           mem.OrganizationMember.UserID.String(),
		Name:         mem.Username,
		FullName:     mem.Name,
		Email:        mem.Email,
		LoginType:    string(user.LoginType),
		RBACRoles:    ownerRoles,
		SSHPublicKey: publicKey,
		Groups:       groupNames,
	}
	return nil
}

func (r *DynamicRenderer) Close() {
	r.once.Do(r.close)
}

func ProvisionerVersionSupportsDynamicParameters(version string) bool {
	major, minor, err := apiversion.Parse(version)
	// If the api version is not valid or less than 1.6, we need to use the static parameters
	useStaticParams := err != nil || major < 1 || (major == 1 && minor < 6)
	return !useStaticParams
}
