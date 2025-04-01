// Package wsbuilder provides the Builder object, which encapsulates the common business logic of inserting a new
// workspace build into the database.
package wsbuilder

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/provisioner/terraform/tfparse"
	"github.com/coder/coder/v2/provisionersdk"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
)

// Builder encapsulates the business logic of inserting a new workspace build into the database.
//
// Builder follows the so-called "Builder" pattern where options that customize the kind of build you get return
// a new instance of the Builder with the option applied.
//
// Example:
//
// b = wsbuilder.New(workspace, transition).VersionID(vID).Initiator(me)
// build, job, err := b.Build(...)
type Builder struct {
	// settings that control the kind of build you get
	workspace        database.Workspace
	trans            database.WorkspaceTransition
	version          versionTarget
	state            stateTarget
	logLevel         string
	deploymentValues *codersdk.DeploymentValues

	richParameterValues     []codersdk.WorkspaceBuildParameter
	initiator               uuid.UUID
	reason                  database.BuildReason
	templateVersionPresetID uuid.UUID

	// used during build, makes function arguments less verbose
	ctx   context.Context
	store database.Store

	// cache of objects, so we only fetch once
	template                     *database.Template
	templateVersion              *database.TemplateVersion
	templateVersionJob           *database.ProvisionerJob
	templateVersionParameters    *[]database.TemplateVersionParameter
	templateVersionVariables     *[]database.TemplateVersionVariable
	templateVersionWorkspaceTags *[]database.TemplateVersionWorkspaceTag
	lastBuild                    *database.WorkspaceBuild
	lastBuildErr                 *error
	lastBuildParameters          *[]database.WorkspaceBuildParameter
	lastBuildJob                 *database.ProvisionerJob
	parameterNames               *[]string
	parameterValues              *[]string

	prebuild bool

	verifyNoLegacyParametersOnce bool
}

type Option func(Builder) Builder

// versionTarget expresses how to determine the template version for the build.
//
// The zero value of this struct means to use the version from the last build.  If there is no last build,
// the build will fail.
//
// setting active: true means to use the active version from the template.
//
// setting specific to a non-nil value means to use the provided template version ID.
//
// active and specific are mutually exclusive and setting them both results in undefined behavior.
type versionTarget struct {
	active   bool
	specific *uuid.UUID
}

// stateTarget expresses how to determine the provisioner state for the build.
//
// The zero value of this struct means to use state from the last build.  If there is no last build, no state is
// provided (i.e. first build on a newly created workspace).
//
// setting orphan: true means not to send any state.  This can be used to deleted orphaned workspaces
//
// setting explicit to a non-nil value means to use the provided state
//
// orphan and explicit are mutually exclusive and setting them both results in undefined behavior.
type stateTarget struct {
	orphan   bool
	explicit *[]byte
}

func New(w database.Workspace, t database.WorkspaceTransition) Builder {
	return Builder{workspace: w, trans: t}
}

// Methods that customize the build are public, have a struct receiver and return a new Builder.

func (b Builder) VersionID(v uuid.UUID) Builder {
	// nolint: revive
	b.version = versionTarget{specific: &v}
	return b
}

func (b Builder) ActiveVersion() Builder {
	// nolint: revive
	b.version = versionTarget{active: true}
	return b
}

func (b Builder) State(state []byte) Builder {
	// nolint: revive
	b.state = stateTarget{explicit: &state}
	return b
}

func (b Builder) Orphan() Builder {
	// nolint: revive
	b.state = stateTarget{orphan: true}
	return b
}

func (b Builder) LogLevel(l string) Builder {
	// nolint: revive
	b.logLevel = l
	return b
}

func (b Builder) DeploymentValues(dv *codersdk.DeploymentValues) Builder {
	// nolint: revive
	b.deploymentValues = dv
	return b
}

func (b Builder) Initiator(u uuid.UUID) Builder {
	// nolint: revive
	b.initiator = u
	return b
}

func (b Builder) Reason(r database.BuildReason) Builder {
	// nolint: revive
	b.reason = r
	return b
}

func (b Builder) RichParameterValues(p []codersdk.WorkspaceBuildParameter) Builder {
	// nolint: revive
	b.richParameterValues = p
	return b
}

func (b Builder) Prebuild() Builder {
	// nolint: revive
	b.prebuild = true
	return b
}

// SetLastWorkspaceBuildInTx prepopulates the Builder's cache with the last workspace build.  This allows us
// to avoid a repeated database query when the Builder's caller also needs the workspace build, e.g. auto-start &
// auto-stop.
//
// CAUTION: only call this method from within a database transaction with RepeatableRead isolation.  This transaction
// MUST be the database.Store you call Build() with.
func (b Builder) SetLastWorkspaceBuildInTx(build *database.WorkspaceBuild) Builder {
	// nolint: revive
	b.lastBuild = build
	return b
}

// SetLastWorkspaceBuildJobInTx prepopulates the Builder's cache with the last workspace build job.  This allows us
// to avoid a repeated database query when the Builder's caller also needs the workspace build job, e.g. auto-start &
// auto-stop.
//
// CAUTION: only call this method from within a database transaction with RepeatableRead isolation.  This transaction
// MUST be the database.Store you call Build() with.
func (b Builder) SetLastWorkspaceBuildJobInTx(job *database.ProvisionerJob) Builder {
	// nolint: revive
	b.lastBuildJob = job
	return b
}

func (b Builder) TemplateVersionPresetID(id uuid.UUID) Builder {
	// nolint: revive
	b.templateVersionPresetID = id
	return b
}

type BuildError struct {
	// Status is a suitable HTTP status code
	Status  int
	Message string
	Wrapped error
}

func (e BuildError) Error() string {
	return e.Wrapped.Error()
}

func (e BuildError) Unwrap() error {
	return e.Wrapped
}

// Build computes and inserts a new workspace build into the database.  If authFunc is provided, it also performs
// authorization preflight checks.
func (b *Builder) Build(
	ctx context.Context,
	store database.Store,
	authFunc func(action policy.Action, object rbac.Objecter) bool,
	auditBaggage audit.WorkspaceBuildBaggage,
) (
	*database.WorkspaceBuild, *database.ProvisionerJob, []database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow, error,
) {
	var err error
	b.ctx, err = audit.BaggageToContext(ctx, auditBaggage)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("create audit baggage: %w", err)
	}

	// Run the build in a transaction with RepeatableRead isolation, and retries.
	// RepeatableRead isolation ensures that we get a consistent view of the database while
	// computing the new build.  This simplifies the logic so that we do not need to worry if
	// later reads are consistent with earlier ones.
	var workspaceBuild *database.WorkspaceBuild
	var provisionerJob *database.ProvisionerJob
	var provisionerDaemons []database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow
	err = database.ReadModifyUpdate(store, func(tx database.Store) error {
		var err error
		b.store = tx
		workspaceBuild, provisionerJob, provisionerDaemons, err = b.buildTx(authFunc)
		return err
	})
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("build tx: %w", err)
	}
	return workspaceBuild, provisionerJob, provisionerDaemons, nil
}

// buildTx contains the business logic of computing a new build.  Attributes of the new database objects are computed
// in a functional style, rather than imperative, to emphasize the logic of how they are defined.  A simple cache
// of database-fetched objects is stored on the struct to ensure we only fetch things once, even if they are used in
// the calculation of multiple attributes.
//
// In order to utilize this cache, the functions that compute build attributes use a pointer receiver type.
func (b *Builder) buildTx(authFunc func(action policy.Action, object rbac.Objecter) bool) (
	*database.WorkspaceBuild, *database.ProvisionerJob, []database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow, error,
) {
	if authFunc != nil {
		err := b.authorize(authFunc)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	err := b.checkTemplateVersionMatchesTemplate()
	if err != nil {
		return nil, nil, nil, err
	}
	err = b.checkTemplateJobStatus()
	if err != nil {
		return nil, nil, nil, err
	}
	err = b.checkRunningBuild()
	if err != nil {
		return nil, nil, nil, err
	}

	template, err := b.getTemplate()
	if err != nil {
		return nil, nil, nil, BuildError{http.StatusInternalServerError, "failed to fetch template", err}
	}

	templateVersionJob, err := b.getTemplateVersionJob()
	if err != nil {
		return nil, nil, nil, BuildError{
			http.StatusInternalServerError, "failed to fetch template version job", err,
		}
	}

	// if we haven't been told specifically who initiated, default to owner
	if b.initiator == uuid.Nil {
		b.initiator = b.workspace.OwnerID
	}
	// default reason is initiator
	if b.reason == "" {
		b.reason = database.BuildReasonInitiator
	}

	workspaceBuildID := uuid.New()
	input, err := json.Marshal(provisionerdserver.WorkspaceProvisionJob{
		WorkspaceBuildID: workspaceBuildID,
		LogLevel:         b.logLevel,
		Prebuild:         b.prebuild,
	})
	if err != nil {
		return nil, nil, nil, BuildError{
			http.StatusInternalServerError,
			"marshal provision job",
			err,
		}
	}
	traceMetadataRaw, err := json.Marshal(tracing.MetadataFromContext(b.ctx))
	if err != nil {
		return nil, nil, nil, BuildError{http.StatusInternalServerError, "marshal metadata", err}
	}

	tags, err := b.getProvisionerTags()
	if err != nil {
		return nil, nil, nil, err // already wrapped BuildError
	}

	now := dbtime.Now()
	provisionerJob, err := b.store.InsertProvisionerJob(b.ctx, database.InsertProvisionerJobParams{
		ID:             uuid.New(),
		CreatedAt:      now,
		UpdatedAt:      now,
		InitiatorID:    b.initiator,
		OrganizationID: template.OrganizationID,
		Provisioner:    template.Provisioner,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		StorageMethod:  templateVersionJob.StorageMethod,
		FileID:         templateVersionJob.FileID,
		Input:          input,
		Tags:           tags,
		TraceMetadata: pqtype.NullRawMessage{
			Valid:      true,
			RawMessage: traceMetadataRaw,
		},
	})
	if err != nil {
		return nil, nil, nil, BuildError{http.StatusInternalServerError, "insert provisioner job", err}
	}

	// nolint:gocritic // The user performing this request may not have permission
	// to read all provisioner daemons. We need to retrieve the eligible
	// provisioner daemons for this job to show in the UI if there is no
	// matching provisioner daemon.
	provisionerDaemons, err := b.store.GetEligibleProvisionerDaemonsByProvisionerJobIDs(dbauthz.AsSystemReadProvisionerDaemons(b.ctx), []uuid.UUID{provisionerJob.ID})
	if err != nil {
		// NOTE: we do **not** want to fail a workspace build if we fail to
		// retrieve provisioner daemons. This is just to show in the UI if there
		// is no matching provisioner daemon for the job.
		provisionerDaemons = []database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}
	}

	templateVersionID, err := b.getTemplateVersionID()
	if err != nil {
		return nil, nil, nil, BuildError{http.StatusInternalServerError, "compute template version ID", err}
	}
	buildNum, err := b.getBuildNumber()
	if err != nil {
		return nil, nil, nil, BuildError{http.StatusInternalServerError, "compute build number", err}
	}
	state, err := b.getState()
	if err != nil {
		return nil, nil, nil, BuildError{http.StatusInternalServerError, "compute build state", err}
	}

	var workspaceBuild database.WorkspaceBuild
	err = b.store.InTx(func(store database.Store) error {
		err = store.InsertWorkspaceBuild(b.ctx, database.InsertWorkspaceBuildParams{
			ID:                workspaceBuildID,
			CreatedAt:         now,
			UpdatedAt:         now,
			WorkspaceID:       b.workspace.ID,
			TemplateVersionID: templateVersionID,
			BuildNumber:       buildNum,
			ProvisionerState:  state,
			InitiatorID:       b.initiator,
			Transition:        b.trans,
			JobID:             provisionerJob.ID,
			Reason:            b.reason,
			Deadline:          time.Time{}, // set by provisioner upon completion
			MaxDeadline:       time.Time{}, // set by provisioner upon completion
			TemplateVersionPresetID: uuid.NullUUID{
				UUID:  b.templateVersionPresetID,
				Valid: b.templateVersionPresetID != uuid.Nil,
			},
		})
		if err != nil {
			code := http.StatusInternalServerError
			if rbac.IsUnauthorizedError(err) {
				code = http.StatusForbidden
			} else if database.IsUniqueViolation(err) {
				// Concurrent builds may result in duplicate
				// workspace_builds_workspace_id_build_number_key.
				code = http.StatusConflict
			}
			return BuildError{code, "insert workspace build", err}
		}

		names, values, err := b.getParameters()
		if err != nil {
			// getParameters already wraps errors in BuildError
			return err
		}

		err = store.InsertWorkspaceBuildParameters(b.ctx, database.InsertWorkspaceBuildParametersParams{
			WorkspaceBuildID: workspaceBuildID,
			Name:             names,
			Value:            values,
		})
		if err != nil {
			return BuildError{http.StatusInternalServerError, "insert workspace build parameters: %w", err}
		}

		workspaceBuild, err = store.GetWorkspaceBuildByID(b.ctx, workspaceBuildID)
		if err != nil {
			return BuildError{http.StatusInternalServerError, "get workspace build", err}
		}

		return nil
	}, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	return &workspaceBuild, &provisionerJob, provisionerDaemons, nil
}

func (b *Builder) getTemplate() (*database.Template, error) {
	if b.template != nil {
		return b.template, nil
	}
	t, err := b.store.GetTemplateByID(b.ctx, b.workspace.TemplateID)
	if err != nil {
		return nil, xerrors.Errorf("get template %s: %w", b.workspace.TemplateID, err)
	}
	b.template = &t
	return b.template, nil
}

func (b *Builder) getTemplateVersionJob() (*database.ProvisionerJob, error) {
	if b.templateVersionJob != nil {
		return b.templateVersionJob, nil
	}
	v, err := b.getTemplateVersion()
	if err != nil {
		return nil, xerrors.Errorf("get template version so we can get provisioner job: %w", err)
	}
	j, err := b.store.GetProvisionerJobByID(b.ctx, v.JobID)
	if err != nil {
		return nil, xerrors.Errorf("get template provisioner job %s: %w", v.JobID, err)
	}
	b.templateVersionJob = &j
	return b.templateVersionJob, err
}

func (b *Builder) getTemplateVersion() (*database.TemplateVersion, error) {
	if b.templateVersion != nil {
		return b.templateVersion, nil
	}
	id, err := b.getTemplateVersionID()
	if err != nil {
		return nil, xerrors.Errorf("get template version ID so we can get version: %w", err)
	}
	v, err := b.store.GetTemplateVersionByID(b.ctx, id)
	if err != nil {
		return nil, xerrors.Errorf("get template version %s: %w", id, err)
	}
	b.templateVersion = &v
	return b.templateVersion, err
}

func (b *Builder) getTemplateVersionID() (uuid.UUID, error) {
	if b.version.specific != nil {
		return *b.version.specific, nil
	}
	if b.version.active {
		t, err := b.getTemplate()
		if err != nil {
			return uuid.Nil, xerrors.Errorf("get template so we can get active version: %w", err)
		}
		return t.ActiveVersionID, nil
	}
	// default is prior version
	bld, err := b.getLastBuild()
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get last build so we can get version: %w", err)
	}
	return bld.TemplateVersionID, nil
}

func (b *Builder) getLastBuild() (*database.WorkspaceBuild, error) {
	if b.lastBuild != nil {
		return b.lastBuild, nil
	}
	// last build might not exist, so we also store the error to prevent repeated queries
	// for a non-existing build
	if b.lastBuildErr != nil {
		return nil, *b.lastBuildErr
	}
	bld, err := b.store.GetLatestWorkspaceBuildByWorkspaceID(b.ctx, b.workspace.ID)
	if err != nil {
		err = xerrors.Errorf("get workspace %s last build: %w", b.workspace.ID, err)
		b.lastBuildErr = &err
		return nil, err
	}
	b.lastBuild = &bld
	return b.lastBuild, nil
}

func (b *Builder) getBuildNumber() (int32, error) {
	bld, err := b.getLastBuild()
	if xerrors.Is(err, sql.ErrNoRows) {
		// first build!
		return 1, nil
	}
	if err != nil {
		return 0, xerrors.Errorf("get last build to compute build number: %w", err)
	}
	return bld.BuildNumber + 1, nil
}

func (b *Builder) getState() ([]byte, error) {
	if b.state.orphan {
		// Orphan means empty state.
		return nil, nil
	}
	if b.state.explicit != nil {
		return *b.state.explicit, nil
	}
	// Default is to use state from prior build
	bld, err := b.getLastBuild()
	if xerrors.Is(err, sql.ErrNoRows) {
		// last build does not exist, which implies empty state
		return nil, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get last build to get state: %w", err)
	}
	return bld.ProvisionerState, nil
}

func (b *Builder) getParameters() (names, values []string, err error) {
	if b.parameterNames != nil {
		return *b.parameterNames, *b.parameterValues, nil
	}

	templateVersionParameters, err := b.getTemplateVersionParameters()
	if err != nil {
		return nil, nil, BuildError{http.StatusInternalServerError, "failed to fetch template version parameters", err}
	}
	lastBuildParameters, err := b.getLastBuildParameters()
	if err != nil {
		return nil, nil, BuildError{http.StatusInternalServerError, "failed to fetch last build parameters", err}
	}
	err = b.verifyNoLegacyParameters()
	if err != nil {
		return nil, nil, BuildError{http.StatusBadRequest, "Unable to build workspace with unsupported parameters", err}
	}
	resolver := codersdk.ParameterResolver{
		Rich: db2sdk.WorkspaceBuildParameters(lastBuildParameters),
	}
	for _, templateVersionParameter := range templateVersionParameters {
		tvp, err := db2sdk.TemplateVersionParameter(templateVersionParameter)
		if err != nil {
			return nil, nil, BuildError{http.StatusInternalServerError, "failed to convert template version parameter", err}
		}
		value, err := resolver.ValidateResolve(
			tvp,
			b.findNewBuildParameterValue(templateVersionParameter.Name),
		)
		if err != nil {
			// At this point, we've queried all the data we need from the database,
			// so the only errors are problems with the request (missing data, failed
			// validation, immutable parameters, etc.)
			return nil, nil, BuildError{http.StatusBadRequest, fmt.Sprintf("Unable to validate parameter %q", templateVersionParameter.Name), err}
		}
		names = append(names, templateVersionParameter.Name)
		values = append(values, value)
	}

	b.parameterNames = &names
	b.parameterValues = &values
	return names, values, nil
}

func (b *Builder) findNewBuildParameterValue(name string) *codersdk.WorkspaceBuildParameter {
	for _, v := range b.richParameterValues {
		if v.Name == name {
			return &v
		}
	}
	return nil
}

func (b *Builder) getLastBuildParameters() ([]database.WorkspaceBuildParameter, error) {
	if b.lastBuildParameters != nil {
		return *b.lastBuildParameters, nil
	}
	bld, err := b.getLastBuild()
	if xerrors.Is(err, sql.ErrNoRows) {
		// if the build doesn't exist, then clearly there can be no parameters.
		b.lastBuildParameters = &[]database.WorkspaceBuildParameter{}
		return *b.lastBuildParameters, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get last build to get parameters: %w", err)
	}
	values, err := b.store.GetWorkspaceBuildParameters(b.ctx, bld.ID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get last build %s parameters: %w", bld.ID, err)
	}
	b.lastBuildParameters = &values
	return values, nil
}

func (b *Builder) getTemplateVersionParameters() ([]database.TemplateVersionParameter, error) {
	if b.templateVersionParameters != nil {
		return *b.templateVersionParameters, nil
	}
	tvID, err := b.getTemplateVersionID()
	if err != nil {
		return nil, xerrors.Errorf("get template version ID to get parameters: %w", err)
	}
	tvp, err := b.store.GetTemplateVersionParameters(b.ctx, tvID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get template version %s parameters: %w", tvID, err)
	}
	b.templateVersionParameters = &tvp
	return tvp, nil
}

func (b *Builder) getTemplateVersionVariables() ([]database.TemplateVersionVariable, error) {
	if b.templateVersionVariables != nil {
		return *b.templateVersionVariables, nil
	}
	tvID, err := b.getTemplateVersionID()
	if err != nil {
		return nil, xerrors.Errorf("get template version ID to get variables: %w", err)
	}
	tvs, err := b.store.GetTemplateVersionVariables(b.ctx, tvID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get template version %s variables: %w", tvID, err)
	}
	b.templateVersionVariables = &tvs
	return tvs, nil
}

// verifyNoLegacyParameters verifies that initiator can't start the workspace build
// if it uses legacy parameters (database.ParameterSchemas).
func (b *Builder) verifyNoLegacyParameters() error {
	if b.verifyNoLegacyParametersOnce {
		return nil
	}
	b.verifyNoLegacyParametersOnce = true

	// Block starting the workspace with legacy parameters.
	if b.trans != database.WorkspaceTransitionStart {
		return nil
	}

	templateVersionJob, err := b.getTemplateVersionJob()
	if err != nil {
		return xerrors.Errorf("failed to fetch template version job: %w", err)
	}

	parameterSchemas, err := b.store.GetParameterSchemasByJobID(b.ctx, templateVersionJob.ID)
	if xerrors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("failed to get parameter schemas: %w", err)
	}

	if len(parameterSchemas) > 0 {
		return xerrors.Errorf("Legacy parameters in use on this version are not supported anymore. Contact your administrator for assistance.")
	}
	return nil
}

func (b *Builder) getLastBuildJob() (*database.ProvisionerJob, error) {
	if b.lastBuildJob != nil {
		return b.lastBuildJob, nil
	}
	bld, err := b.getLastBuild()
	if err != nil {
		return nil, xerrors.Errorf("get last build to get job: %w", err)
	}
	job, err := b.store.GetProvisionerJobByID(b.ctx, bld.JobID)
	if err != nil {
		return nil, xerrors.Errorf("get build provisioner job %s: %w", bld.JobID, err)
	}
	b.lastBuildJob = &job
	return b.lastBuildJob, nil
}

func (b *Builder) getProvisionerTags() (map[string]string, error) {
	// Step 1: Mutate template version tags
	templateVersionJob, err := b.getTemplateVersionJob()
	if err != nil {
		return nil, BuildError{http.StatusInternalServerError, "failed to fetch template version job", err}
	}
	annotationTags := provisionersdk.MutateTags(b.workspace.OwnerID, templateVersionJob.Tags)

	tags := map[string]string{}
	for name, value := range annotationTags {
		tags[name] = value
	}

	// Step 2: Mutate workspace tags:
	// - Get workspace tags from the template version job
	// - Get template version variables from the template version as they can be
	//   referenced in workspace tags
	// - Get parameters from the workspace build as they can also be referenced
	//   in workspace tags
	// - Evaluate workspace tags given the above inputs
	workspaceTags, err := b.getTemplateVersionWorkspaceTags()
	if err != nil {
		return nil, BuildError{http.StatusInternalServerError, "failed to fetch template version workspace tags", err}
	}
	tvs, err := b.getTemplateVersionVariables()
	if err != nil {
		return nil, BuildError{http.StatusInternalServerError, "failed to fetch template version variables", err}
	}
	varsM := make(map[string]string)
	for _, tv := range tvs {
		// FIXME: do this in Terraform? This is a bit of a hack.
		if tv.Value == "" {
			varsM[tv.Name] = tv.DefaultValue
		} else {
			varsM[tv.Name] = tv.Value
		}
	}
	parameterNames, parameterValues, err := b.getParameters()
	if err != nil {
		return nil, err // already wrapped BuildError
	}
	paramsM := make(map[string]string)
	for i, name := range parameterNames {
		paramsM[name] = parameterValues[i]
	}

	evalCtx := tfparse.BuildEvalContext(varsM, paramsM)
	for _, workspaceTag := range workspaceTags {
		expr, diags := hclsyntax.ParseExpression([]byte(workspaceTag.Value), "expression.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			return nil, BuildError{http.StatusBadRequest, "failed to parse workspace tag value", xerrors.Errorf(diags.Error())}
		}

		val, diags := expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, BuildError{http.StatusBadRequest, "failed to evaluate workspace tag value", xerrors.Errorf(diags.Error())}
		}

		// Do not use "val.AsString()" as it can panic
		str, err := tfparse.CtyValueString(val)
		if err != nil {
			return nil, BuildError{http.StatusBadRequest, "failed to marshal cty.Value as string", err}
		}
		tags[workspaceTag.Key] = str
	}
	return tags, nil
}

func (b *Builder) getTemplateVersionWorkspaceTags() ([]database.TemplateVersionWorkspaceTag, error) {
	if b.templateVersionWorkspaceTags != nil {
		return *b.templateVersionWorkspaceTags, nil
	}

	templateVersion, err := b.getTemplateVersion()
	if err != nil {
		return nil, xerrors.Errorf("get template version: %w", err)
	}

	workspaceTags, err := b.store.GetTemplateVersionWorkspaceTags(b.ctx, templateVersion.ID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get template version workspace tags: %w", err)
	}

	b.templateVersionWorkspaceTags = &workspaceTags
	return *b.templateVersionWorkspaceTags, nil
}

// authorize performs build authorization pre-checks using the provided authFunc
func (b *Builder) authorize(authFunc func(action policy.Action, object rbac.Objecter) bool) error {
	// Doing this up front saves a lot of work if the user doesn't have permission.
	// This is checked again in the dbauthz layer, but the check is cached
	// and will be a noop later.
	var action policy.Action
	switch b.trans {
	case database.WorkspaceTransitionDelete:
		action = policy.ActionDelete
	case database.WorkspaceTransitionStart, database.WorkspaceTransitionStop:
		action = policy.ActionUpdate
	default:
		msg := fmt.Sprintf("Transition %q not supported.", b.trans)
		return BuildError{http.StatusBadRequest, msg, xerrors.New(msg)}
	}
	if !authFunc(action, b.workspace) {
		if authFunc(policy.ActionRead, b.workspace) {
			// If the user can read the workspace, but not delete/create/update. Show
			// a more helpful error. They are allowed to know the workspace exists.
			return BuildError{
				Status:  http.StatusForbidden,
				Message: fmt.Sprintf("You do not have permission to %s this workspace.", action),
				Wrapped: xerrors.New(httpapi.ResourceForbiddenResponse.Detail),
			}
		}
		// We use the same wording as the httpapi to avoid leaking the existence of the workspace
		return BuildError{http.StatusNotFound, httpapi.ResourceNotFoundResponse.Message, xerrors.New(httpapi.ResourceNotFoundResponse.Message)}
	}

	template, err := b.getTemplate()
	if err != nil {
		return BuildError{http.StatusInternalServerError, "failed to fetch template", err}
	}

	// If custom state, deny request since user could be corrupting or leaking
	// cloud state.
	if b.state.explicit != nil || b.state.orphan {
		if !authFunc(policy.ActionUpdate, template.RBACObject()) {
			return BuildError{http.StatusForbidden, "Only template managers may provide custom state", xerrors.New("Only template managers may provide custom state")}
		}
	}

	if b.logLevel != "" && !authFunc(policy.ActionRead, rbac.ResourceDeploymentConfig) {
		return BuildError{
			http.StatusBadRequest,
			"Workspace builds with a custom log level are restricted to administrators only.",
			xerrors.New("Workspace builds with a custom log level are restricted to administrators only."),
		}
	}

	if b.logLevel != "" && b.deploymentValues != nil && !b.deploymentValues.EnableTerraformDebugMode {
		return BuildError{
			http.StatusBadRequest,
			"Terraform debug mode is disabled in the deployment configuration.",
			xerrors.New("Terraform debug mode is disabled in the deployment configuration."),
		}
	}
	return nil
}

func (b *Builder) checkTemplateVersionMatchesTemplate() error {
	template, err := b.getTemplate()
	if err != nil {
		return BuildError{http.StatusInternalServerError, "failed to fetch template", err}
	}
	templateVersion, err := b.getTemplateVersion()
	if xerrors.Is(err, sql.ErrNoRows) {
		return BuildError{http.StatusBadRequest, "template version does not exist", err}
	}
	if err != nil {
		return BuildError{http.StatusInternalServerError, "failed to fetch template version", err}
	}
	if !templateVersion.TemplateID.Valid || templateVersion.TemplateID.UUID != template.ID {
		return BuildError{
			http.StatusBadRequest,
			"template version doesn't match template",
			xerrors.Errorf("templateVersion.TemplateID = %+v, template.ID = %s",
				templateVersion.TemplateID, template.ID),
		}
	}
	return nil
}

func (b *Builder) checkTemplateJobStatus() error {
	templateVersion, err := b.getTemplateVersion()
	if err != nil {
		return BuildError{http.StatusInternalServerError, "failed to fetch template version", err}
	}

	templateVersionJob, err := b.getTemplateVersionJob()
	if err != nil {
		return BuildError{
			http.StatusInternalServerError, "failed to fetch template version job", err,
		}
	}

	templateVersionJobStatus := codersdk.ProvisionerJobStatus(templateVersionJob.JobStatus)
	switch templateVersionJobStatus {
	case codersdk.ProvisionerJobPending, codersdk.ProvisionerJobRunning:
		msg := fmt.Sprintf("The provided template version is %s. Wait for it to complete importing!", templateVersionJobStatus)

		return BuildError{
			http.StatusNotAcceptable,
			msg,
			xerrors.New(msg),
		}
	case codersdk.ProvisionerJobFailed:
		msg := fmt.Sprintf("The provided template version %q has failed to import: %q. You cannot build workspaces with it!", templateVersion.Name, templateVersionJob.Error.String)
		return BuildError{
			http.StatusBadRequest,
			msg,
			xerrors.New(msg),
		}
	case codersdk.ProvisionerJobCanceled:
		msg := fmt.Sprintf("The provided template version %q has failed to import: %q. You cannot build workspaces with it!", templateVersion.Name, templateVersionJob.Error.String)
		return BuildError{
			http.StatusBadRequest,
			msg,
			xerrors.New(msg),
		}
	}
	return nil
}

func (b *Builder) checkRunningBuild() error {
	job, err := b.getLastBuildJob()
	if xerrors.Is(err, sql.ErrNoRows) {
		// no prior build, so it can't be running!
		return nil
	}
	if err != nil {
		return BuildError{http.StatusInternalServerError, "failed to fetch prior build", err}
	}
	if codersdk.ProvisionerJobStatus(job.JobStatus).Active() {
		msg := "A workspace build is already active."
		return BuildError{
			http.StatusConflict,
			msg,
			xerrors.New(msg),
		}
	}
	return nil
}
