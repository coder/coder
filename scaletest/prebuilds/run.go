package prebuilds

import (
	"bytes"
	"context"
	_ "embed"
	"html/template"
	"io"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	template codersdk.Template

	prebuildTotalLatency       time.Duration
	prebuildJobCreationLatency time.Duration
	prebuildJobAcquiredLatency time.Duration

	prebuildDeletionTotalLatency       time.Duration
	prebuildDeletionJobCreationLatency time.Duration
	prebuildDeletionJobAcquiredLatency time.Duration
}

var (
	_ harness.Runnable    = &Runner{}
	_ harness.Cleanable   = &Runner{}
	_ harness.Collectable = &Runner{}
)

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client: client,
		cfg:    cfg,
	}
}

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	reachedBarrier := false
	defer func() {
		if !reachedBarrier {
			r.cfg.SetupBarrier.Done()
		}
	}()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)
	r.client.SetLogger(logger)
	r.client.SetLogBodies(true)

	templateName := "scaletest-prebuilds-template-" + id

	version, err := r.createTemplateVersion(ctx, uuid.Nil, r.cfg.NumPresets, r.cfg.NumPresetPrebuilds)
	if err != nil {
		r.cfg.Metrics.AddError(templateName, "create_template_version")
		return err
	}

	templateReq := codersdk.CreateTemplateRequest{
		Name:        templateName,
		Description: "`coder exp scaletest prebuilds` template",
		VersionID:   version.ID,
	}
	templ, err := r.client.CreateTemplate(ctx, r.cfg.OrganizationID, templateReq)
	if err != nil {
		r.cfg.Metrics.AddError(templateName, "create_template")
		return xerrors.Errorf("create template: %w", err)
	}
	logger.Info(ctx, "created template", slog.F("template_id", templ.ID))

	r.template = templ

	logger.Info(ctx, "waiting for all runners to reach barrier")
	reachedBarrier = true
	r.cfg.SetupBarrier.Done()
	r.cfg.SetupBarrier.Wait()
	logger.Info(ctx, "all runners reached barrier, proceeding with prebuilds test")

	err = r.measureCreation(ctx, logger)
	if err != nil {
		return err
	}

	return nil
}

func (r *Runner) measureCreation(ctx context.Context, logger slog.Logger) error {
	testStartTime := time.Now().UTC()
	const workspacesPollInterval = 500 * time.Millisecond

	targetNumWorkspaces := r.cfg.NumPresets * r.cfg.NumPresetPrebuilds

	workspacesCtx, cancel := context.WithTimeout(ctx, r.cfg.PrebuildWorkspaceTimeout)
	defer cancel()

	tkr := r.cfg.Clock.TickerFunc(workspacesCtx, workspacesPollInterval, func() error {
		workspaces, err := r.client.Workspaces(workspacesCtx, codersdk.WorkspaceFilter{
			Template: r.template.Name,
		})
		if err != nil {
			return xerrors.Errorf("list workspaces: %w", err)
		}

		acquiredCount := 0
		succeededCount := 0

		for _, ws := range workspaces.Workspaces {
			if ws.LatestBuild.Job.Status == codersdk.ProvisionerJobRunning ||
				ws.LatestBuild.Job.Status == codersdk.ProvisionerJobSucceeded {
				acquiredCount++
			}
			if ws.LatestBuild.Job.Status == codersdk.ProvisionerJobSucceeded {
				succeededCount++
			}
		}

		if r.prebuildJobCreationLatency == 0 && len(workspaces.Workspaces) >= targetNumWorkspaces {
			// All jobs created
			r.prebuildJobCreationLatency = time.Since(testStartTime)
			r.cfg.Metrics.RecordJobCreation(r.prebuildJobCreationLatency, r.template.Name)
		}

		if r.prebuildJobAcquiredLatency == 0 && acquiredCount >= targetNumWorkspaces {
			// All jobs acquired
			r.prebuildJobAcquiredLatency = time.Since(testStartTime)
			r.cfg.Metrics.RecordJobAcquired(r.prebuildJobAcquiredLatency, r.template.Name)
		}

		if succeededCount >= targetNumWorkspaces {
			// All jobs succeeded
			r.prebuildTotalLatency = time.Since(testStartTime)
			r.cfg.Metrics.RecordCompletion(r.prebuildTotalLatency, r.template.Name)
			return errTickerDone
		}

		return nil
	}, "waitForPrebuildWorkspaces")
	err := tkr.Wait()
	if !xerrors.Is(err, errTickerDone) {
		r.cfg.Metrics.AddError(r.template.Name, "wait_for_workspaces")
		return xerrors.Errorf("wait for workspaces: %w", err)
	}

	logger.Info(ctx, "all prebuild workspaces created successfully", slog.F("template_name", r.template.Name), slog.F("duration", time.Since(testStartTime).String()))
	return nil
}

func (r *Runner) measureDeletion(ctx context.Context, logger slog.Logger) error {
	deletionStartTime := time.Now().UTC()
	const deletionPollInterval = 500 * time.Millisecond

	targetNumWorkspaces := r.cfg.NumPresets * r.cfg.NumPresetPrebuilds

	deletionCtx, cancel := context.WithTimeout(ctx, r.cfg.PrebuildWorkspaceTimeout)
	defer cancel()

	tkr := r.cfg.Clock.TickerFunc(deletionCtx, deletionPollInterval, func() error {
		workspaces, err := r.client.Workspaces(deletionCtx, codersdk.WorkspaceFilter{
			Template: r.template.Name,
		})
		if err != nil {
			return xerrors.Errorf("list workspaces: %w", err)
		}

		currentCount := len(workspaces.Workspaces)
		deletingCount := 0

		for _, ws := range workspaces.Workspaces {
			if ws.LatestBuild.Transition == codersdk.WorkspaceTransitionDelete {
				if ws.LatestBuild.Job.Status == codersdk.ProvisionerJobRunning ||
					ws.LatestBuild.Job.Status == codersdk.ProvisionerJobSucceeded {
					deletingCount++
				}
			}
		}

		if r.prebuildDeletionJobCreationLatency == 0 && (currentCount < targetNumWorkspaces || deletingCount > 0) {
			r.prebuildDeletionJobCreationLatency = time.Since(deletionStartTime)
			r.cfg.Metrics.RecordDeletionJobCreation(r.prebuildDeletionJobCreationLatency, r.template.Name)
		}

		if r.prebuildDeletionJobAcquiredLatency == 0 && deletingCount > 0 {
			r.prebuildDeletionJobAcquiredLatency = time.Since(deletionStartTime)
			r.cfg.Metrics.RecordDeletionJobAcquired(r.prebuildDeletionJobAcquiredLatency, r.template.Name)
		}

		if currentCount == 0 {
			r.prebuildDeletionTotalLatency = time.Since(deletionStartTime)
			r.cfg.Metrics.RecordDeletionCompletion(r.prebuildDeletionTotalLatency, r.template.Name)
			return errTickerDone
		}

		return nil
	}, "waitForPrebuildWorkspacesDeletion")
	err := tkr.Wait()
	if !xerrors.Is(err, errTickerDone) {
		r.cfg.Metrics.AddError(r.template.Name, "wait_for_workspace_deletion")
		return xerrors.Errorf("wait for workspace deletion: %w", err)
	}

	logger.Info(ctx, "all prebuild workspaces deleted successfully", slog.F("template_name", r.template.Name), slog.F("duration", time.Since(deletionStartTime).String()))
	return nil
}

func (r *Runner) createTemplateVersion(ctx context.Context, templateID uuid.UUID, numPresets, numPresetPrebuilds int) (codersdk.TemplateVersion, error) {
	tarData, err := TemplateTarData(numPresets, numPresetPrebuilds)
	if err != nil {
		return codersdk.TemplateVersion{}, xerrors.Errorf("create prebuilds template tar: %w", err)
	}
	uploadResp, err := r.client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(tarData))
	if err != nil {
		return codersdk.TemplateVersion{}, xerrors.Errorf("upload prebuilds template tar: %w", err)
	}

	versionReq := codersdk.CreateTemplateVersionRequest{
		TemplateID:    templateID,
		FileID:        uploadResp.ID,
		Message:       "Template version for scaletest prebuilds",
		StorageMethod: codersdk.ProvisionerStorageMethodFile,
		Provisioner:   codersdk.ProvisionerTypeTerraform,
	}
	version, err := r.client.CreateTemplateVersion(ctx, r.cfg.OrganizationID, versionReq)
	if err != nil {
		return codersdk.TemplateVersion{}, xerrors.Errorf("create template version: %w", err)
	}
	if version.MatchedProvisioners != nil && version.MatchedProvisioners.Count == 0 {
		return codersdk.TemplateVersion{}, xerrors.Errorf("no provisioners matched for template version")
	}

	const pollInterval = 2 * time.Second
	versionCtx, cancel := context.WithTimeout(ctx, r.cfg.TemplateVersionJobTimeout)
	defer cancel()

	tkr := r.cfg.Clock.TickerFunc(versionCtx, pollInterval, func() error {
		version, err := r.client.TemplateVersion(versionCtx, version.ID)
		if err != nil {
			return xerrors.Errorf("get template version: %w", err)
		}
		switch version.Job.Status {
		case codersdk.ProvisionerJobSucceeded:
			return errTickerDone
		case codersdk.ProvisionerJobPending, codersdk.ProvisionerJobRunning:
			return nil
		default:
			return xerrors.Errorf("template version provisioning failed: status %s", version.Job.Status)
		}
	})
	err = tkr.Wait()
	if !xerrors.Is(err, errTickerDone) {
		return codersdk.TemplateVersion{}, xerrors.Errorf("wait for template version provisioning: %w", err)
	}
	return version, nil
}

var errTickerDone = xerrors.New("done")

func (r *Runner) Cleanup(ctx context.Context, _ string, logs io.Writer) error {
	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)

	reachedDeletionBarrier := false
	defer func() {
		if !reachedDeletionBarrier {
			r.cfg.DeletionBarrier.Done()
		}
	}()

	version, err := r.createTemplateVersion(ctx, r.template.ID, 0, 0)
	if err != nil {
		r.cfg.Metrics.AddError(r.template.Name, "create_empty_template_version")
		return xerrors.Errorf("create empty template version for deletion: %w", err)
	}

	err = r.client.UpdateActiveTemplateVersion(context.Background(), r.template.ID, codersdk.UpdateActiveTemplateVersion{
		ID: version.ID,
	})
	if err != nil {
		r.cfg.Metrics.AddError(r.template.Name, "update_active_template_version")
		return xerrors.Errorf("update active template version to empty for deletion: %w", err)
	}

	logger.Info(ctx, "waiting for all runners to reach deletion barrier")
	reachedDeletionBarrier = true
	r.cfg.DeletionBarrier.Done()
	r.cfg.DeletionBarrier.Wait()
	logger.Info(ctx, "all runners reached deletion barrier, proceeding with prebuild deletion")

	err = r.measureDeletion(ctx, logger)
	if err != nil {
		return err
	}

	logger.Info(ctx, "deleting template", slog.F("template_name", r.template.Name))

	err = r.client.DeleteTemplate(ctx, r.template.ID)
	if err != nil {
		return xerrors.Errorf("delete template: %w", err)
	}

	logger.Info(ctx, "template deleted successfully", slog.F("template_name", r.template.Name))
	return nil
}

const (
	PrebuildsTotalLatencyMetric              = "prebuild_total_latency_ms"
	PrebuildJobCreationLatencyMetric         = "prebuild_job_creation_latency_ms"
	PrebuildJobAcquiredLatencyMetric         = "prebuild_job_acquired_latency_ms"
	PrebuildDeletionTotalLatencyMetric       = "prebuild_deletion_total_latency_ms"
	PrebuildDeletionJobCreationLatencyMetric = "prebuild_deletion_job_creation_latency_ms"
	PrebuildDeletionJobAcquiredLatencyMetric = "prebuild_deletion_job_acquired_latency_ms"
)

func (r *Runner) GetMetrics() map[string]any {
	return map[string]any{
		PrebuildsTotalLatencyMetric:              r.prebuildTotalLatency.Milliseconds(),
		PrebuildJobCreationLatencyMetric:         r.prebuildJobCreationLatency.Milliseconds(),
		PrebuildJobAcquiredLatencyMetric:         r.prebuildJobAcquiredLatency.Milliseconds(),
		PrebuildDeletionTotalLatencyMetric:       r.prebuildDeletionTotalLatency.Milliseconds(),
		PrebuildDeletionJobCreationLatencyMetric: r.prebuildDeletionJobCreationLatency.Milliseconds(),
		PrebuildDeletionJobAcquiredLatencyMetric: r.prebuildDeletionJobAcquiredLatency.Milliseconds(),
	}
}

//go:embed tf/main.tf.tpl
var templateContent string

func TemplateTarData(numPresets, numPresetPrebuilds int) ([]byte, error) {
	tmpl, err := template.New("prebuilds-template").Parse(templateContent)
	if err != nil {
		return nil, err
	}
	var result strings.Builder
	err = tmpl.Execute(&result, map[string]int{
		"NumPresets":         numPresets,
		"NumPresetPrebuilds": numPresetPrebuilds,
	})
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{
		"main.tf": []byte(result.String()),
	}
	tarBytes, err := loadtestutil.CreateTarFromFiles(files)
	if err != nil {
		return nil, err
	}
	return tarBytes, nil
}
