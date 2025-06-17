package dynamicparameters

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/preview"

	"github.com/hashicorp/hcl/v2"
)

type Loader struct {
	templateVersionID uuid.UUID

	// cache of objects
	templateVersion *database.TemplateVersion
	job             *database.ProvisionerJob
	terraformValues *database.TemplateVersionTerraformValue
}

func New(ctx context.Context, versionID uuid.UUID) *Loader {
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
		return xerrors.Errorf("job has not completed")
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

func (r *Loader) Renderer(ctx context.Context, cache *files.Cache) (any, error) {
	if !r.loaded() {
		return nil, xerrors.New("Load() must be called before Renderer()")
	}

	// If they can read the template version, then they can read the file.
	fileCtx := dbauthz.AsFileReader(ctx)
	templateFS, err := cache.Acquire(fileCtx, r.job.FileID)
	if err != nil {
		return nil, xerrors.Errorf("acquire template file: %w", err)
	}

}

func (r *Loader) Render(ctx context.Context, ownerID uuid.UUID, values map[string]string) (*preview.Output, hcl.Diagnostics) {

	return nil, nil
}

func ProvisionerVersionSupportsDynamicParameters(version string) bool {
	major, minor, err := apiversion.Parse(version)
	// If the api version is not valid or less than 1.6, we need to use the static parameters
	useStaticParams := err != nil || major < 1 || (major == 1 && minor < 6)
	return !useStaticParams
}
