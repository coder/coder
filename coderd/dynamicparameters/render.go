package dynamicparameters

import (
	"context"
	"database/sql"
	"io/fs"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
)

// RenderResult is the structured output of Renderer.Render. The outer
// pointer is always non-nil; inner fields may be nil.
// SecretRequirements is nil when no coder_secret blocks are declared,
// when fetch was forbidden, or when fetch failed. Output may be nil
// when underlying rendering fails (matches preview.Preview's existing
// convention).
type RenderResult struct {
	Output             *preview.Output
	SecretRequirements []codersdk.SecretRequirementStatus
}

// Renderer is able to execute and evaluate terraform with the given inputs.
// It may use the database to fetch additional state, such as a user's groups,
// roles, etc. Therefore, it requires an authenticated `ctx`.
//
// 'Close()' **must** be called once the renderer is no longer needed.
// Forgetting to do so will result in a memory leak.
type Renderer interface {
	Render(ctx context.Context, ownerID uuid.UUID, values map[string]string, opts ...RenderOption) (*RenderResult, hcl.Diagnostics)
	Close()
}

var ErrTemplateVersionNotReady = xerrors.New("template version job not finished")

// RenderOption configures optional behavior for Renderer.Render.
type RenderOption func(*renderOptions)

type renderOptions struct {
	includeSecretRequirements bool
}

// IncludeSecretRequirements returns structured secret-requirement statuses and
// diagnostics for the rendered template.
func IncludeSecretRequirements() RenderOption {
	return func(o *renderOptions) {
		o.includeSecretRequirements = true
	}
}

// Diagnostic extra codes for secret-requirement validation.
const (
	DiagCodeMissingSecret             = "missing_secret"
	DiagCodeOwnerSecretsFetchFailed   = "owner_secrets_fetch_failed"
	DiagCodeSecretValidationForbidden = "secret_validation_forbidden"
)

// loader is used to load the necessary coder objects for rendering a template
// version's parameters. The output is a Renderer, which is the object that uses
// the cached objects to render the template version's parameters.
type loader struct {
	templateVersionID uuid.UUID
	logger            slog.Logger

	// cache of objects
	templateVersion        *database.TemplateVersion
	job                    *database.ProvisionerJob
	terraformValues        *database.TemplateVersionTerraformValue
	templateVariableValues *[]database.TemplateVersionVariable
}

// Prepare is the entrypoint for this package. It loads the necessary objects &
// files from the database and returns a Renderer that can be used to render the
// template version's parameters.
func Prepare(ctx context.Context, db database.Store, cache files.FileAcquirer, versionID uuid.UUID, options ...func(r *loader)) (Renderer, error) {
	l := &loader{
		templateVersionID: versionID,
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

// WithLogger sets the logger used by the renderer.
func WithLogger(logger slog.Logger) func(r *loader) {
	return func(r *loader) {
		r.logger = logger
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
		data:              r,
		templateFS:        templateFS,
		db:                db,
		logger:            r.logger,
		ownerErrors:       make(map[uuid.UUID]error),
		ownerSecretErrors: make(map[uuid.UUID]error),
		close:             cache.Close,
		tfvarValues:       tfVarValues,
	}, nil
}

type dynamicRenderer struct {
	db         database.Store
	data       *loader
	templateFS fs.FS
	logger     slog.Logger

	ownerErrors  map[uuid.UUID]error
	currentOwner *previewtypes.WorkspaceOwner

	// ownerSecretErrors caches NotAuthorized denials per owner.
	ownerSecretErrors map[uuid.UUID]error

	tfvarValues map[string]cty.Value

	once  sync.Once
	close func()
}

func (r *dynamicRenderer) Render(ctx context.Context, ownerID uuid.UUID, values map[string]string, opts ...RenderOption) (*RenderResult, hcl.Diagnostics) {
	options := renderOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	// Always start with the cached error, if we have one.
	ownerErr := r.ownerErrors[ownerID]
	if ownerErr == nil {
		ownerErr = r.getWorkspaceOwnerData(ctx, ownerID)
	}

	if ownerErr != nil || r.currentOwner == nil {
		r.ownerErrors[ownerID] = ownerErr
		return &RenderResult{}, hcl.Diagnostics{
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
		// Leave Logger nil so preview discards parser logs. Returning
		// those logs to callers would be useful, but they may be large.
	}

	output, diags := preview.Preview(ctx, input, r.templateFS)
	if output == nil {
		return &RenderResult{}, diags
	}

	var secretRequirements []codersdk.SecretRequirementStatus
	if options.includeSecretRequirements && len(output.SecretRequirements) > 0 {
		var secretDiags hcl.Diagnostics
		secretRequirements, secretDiags = r.checkSecretRequirements(ctx, ownerID, output.SecretRequirements)
		diags = diags.Extend(secretDiags)
	}

	return &RenderResult{
		Output:             output,
		SecretRequirements: secretRequirements,
	}, diags
}

// checkSecretRequirements returns structured requirement statuses. Callers
// without user_secret:read on the owner get a single
// secret_validation_forbidden warning instead, to avoid leaking the target's
// secret names via structured status presence.
func (r *dynamicRenderer) checkSecretRequirements(ctx context.Context, ownerID uuid.UUID, reqs []previewtypes.SecretRequirement) ([]codersdk.SecretRequirementStatus, hcl.Diagnostics) {
	secrets, err := r.getOwnerSecrets(ctx, ownerID)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			// Warning keeps the Create Workspace button enabled.
			return nil, hcl.Diagnostics{{
				Severity: hcl.DiagWarning,
				Summary:  "Cannot validate secret requirements",
				Detail:   "You are not permitted to read secret metadata for this user. The workspace may fail to build if required secrets are not set.",
				Extra: previewtypes.DiagnosticExtra{
					Code: DiagCodeSecretValidationForbidden,
				},
			}}
		}
		r.logger.Warn(ctx, "failed to fetch owner secrets for secret-requirement validation",
			slog.F("owner_id", ownerID),
			slog.Error(err),
		)
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Failed to fetch owner secrets",
			Detail:   "Could not validate template secret requirements. Please try again.",
			Extra: previewtypes.DiagnosticExtra{
				Code: DiagCodeOwnerSecretsFetchFailed,
			},
		}}
	}

	envSet := make(map[string]struct{}, len(secrets))
	fileSet := make(map[string]struct{}, len(secrets))
	for _, s := range secrets {
		if s.EnvName != "" {
			envSet[s.EnvName] = struct{}{}
		}
		if s.FilePath != "" {
			fileSet[s.FilePath] = struct{}{}
		}
	}

	statuses := make([]codersdk.SecretRequirementStatus, 0, len(reqs))
	type secretRequirementDedupKey struct {
		env  string
		file string
	}
	seen := make(map[secretRequirementDedupKey]int, len(reqs))
	for _, req := range reqs {
		kind := secretRequirementKind(req.Env, req.File)
		if kind == "" {
			// Defensive: SecretFromBlock should reject invalid inputs upstream.
			continue
		}

		var env string
		var file string
		satisfied := false
		switch kind {
		case secretRequirementKindEnv:
			env = req.Env
			_, satisfied = envSet[req.Env]
		case secretRequirementKindFile:
			file = req.File
			_, satisfied = fileSet[req.File]
		}

		// Dedup by Env/File. On collision, keep the
		// lexicographically smallest non-empty HelpMessage. This is
		// deterministic across runs; preview's SortSecretRequirements
		// sorts on (Env, File) and does not guarantee a stable order
		// when multiple coder_secret blocks declare the same value, so
		// we cannot rely on "first source wins."
		key := secretRequirementDedupKey{
			env:  env,
			file: file,
		}
		if i, ok := seen[key]; ok {
			statuses[i].Satisfied = statuses[i].Satisfied || satisfied
			if req.HelpMessage != "" && (statuses[i].HelpMessage == "" || req.HelpMessage < statuses[i].HelpMessage) {
				statuses[i].HelpMessage = req.HelpMessage
			}
			continue
		}
		seen[key] = len(statuses)
		statuses = append(statuses, codersdk.SecretRequirementStatus{
			Env:         env,
			File:        file,
			HelpMessage: req.HelpMessage,
			Satisfied:   satisfied,
		})
	}
	return statuses, nil
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

// getOwnerSecrets fetches the owner's secrets under the caller's auth
// context. Only NotAuthorized denials are cached; successes re-fetch so
// newly-created secrets are picked up on the next render.
func (r *dynamicRenderer) getOwnerSecrets(ctx context.Context, ownerID uuid.UUID) ([]database.ListUserSecretsRow, error) {
	if err, cached := r.ownerSecretErrors[ownerID]; cached {
		return nil, err
	}
	rows, err := r.db.ListUserSecrets(ctx, ownerID)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			r.ownerSecretErrors[ownerID] = err
		}
		return nil, err
	}
	return rows, nil
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
