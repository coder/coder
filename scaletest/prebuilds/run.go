package prebuilds

import (
	"bytes"
	"context"
	_ "embed"
	"html/template"
	"io"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	template codersdk.Template
}

var (
	_ harness.Runnable  = &Runner{}
	_ harness.Cleanable = &Runner{}
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

	reachedSetupBarrier := false
	reachedCreationBarrier := false
	reachedDeletionBarrier := false
	defer func() {
		if !reachedSetupBarrier {
			r.cfg.SetupBarrier.Done()
		}
		if !reachedCreationBarrier {
			r.cfg.CreationBarrier.Done()
		}
		if !reachedDeletionBarrier {
			r.cfg.DeletionBarrier.Done()
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

	logger.Info(ctx, "waiting for all runners to reach setup barrier")
	reachedSetupBarrier = true
	r.cfg.SetupBarrier.Done()
	r.cfg.SetupBarrier.Wait()
	logger.Info(ctx, "all runners reached setup barrier, proceeding with prebuild creation test")

	err = r.measureCreation(ctx, logger)
	if err != nil {
		return err
	}

	logger.Info(ctx, "waiting for all runners to reach creation barrier")
	reachedCreationBarrier = true
	r.cfg.CreationBarrier.Done()
	r.cfg.CreationBarrier.Wait()
	logger.Info(ctx, "all runners reached creation barrier")

	logger.Info(ctx, "waiting for runner owner to pause prebuilds (deletion setup barrier)")
	r.cfg.DeletionSetupBarrier.Wait()
	logger.Info(ctx, "prebuilds paused, preparing for deletion")

	// Now prepare for deletion by creating an empty template version
	// At this point, prebuilds should be paused by the caller
	logger.Info(ctx, "creating empty template version for deletion")
	emptyVersion, err := r.createTemplateVersion(ctx, r.template.ID, 0, 0)
	if err != nil {
		r.cfg.Metrics.AddError(r.template.Name, "create_empty_template_version")
		return xerrors.Errorf("create empty template version for deletion: %w", err)
	}

	err = r.client.UpdateActiveTemplateVersion(ctx, r.template.ID, codersdk.UpdateActiveTemplateVersion{
		ID: emptyVersion.ID,
	})
	if err != nil {
		r.cfg.Metrics.AddError(r.template.Name, "update_active_template_version")
		return xerrors.Errorf("update active template version to empty for deletion: %w", err)
	}

	logger.Info(ctx, "waiting for all runners to reach deletion barrier")
	reachedDeletionBarrier = true
	r.cfg.DeletionBarrier.Done()
	r.cfg.DeletionBarrier.Wait()
	logger.Info(ctx, "all runners reached deletion barrier, proceeding with prebuild deletion test")

	err = r.measureDeletion(ctx, logger)
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

		createdCount := len(workspaces.Workspaces)
		runningCount := 0
		failedCount := 0
		succeededCount := 0

		for _, ws := range workspaces.Workspaces {
			switch ws.LatestBuild.Job.Status {
			case codersdk.ProvisionerJobRunning:
				runningCount++
			case codersdk.ProvisionerJobFailed, codersdk.ProvisionerJobCanceled:
				failedCount++
			case codersdk.ProvisionerJobSucceeded:
				succeededCount++
			}
		}

		r.cfg.Metrics.SetJobsCreated(createdCount, r.template.Name)
		r.cfg.Metrics.SetJobsRunning(runningCount, r.template.Name)
		r.cfg.Metrics.SetJobsFailed(failedCount, r.template.Name)
		r.cfg.Metrics.SetJobsCompleted(succeededCount, r.template.Name)

		if succeededCount >= targetNumWorkspaces {
			// All jobs succeeded
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
	const (
		deletionPollInterval = 500 * time.Millisecond
		maxDeletionRetries   = 3
	)

	deletionCtx, cancel := context.WithTimeout(ctx, r.cfg.PrebuildWorkspaceTimeout)
	defer cancel()

	// Capture the actual workspace count at the start of the deletion phase.
	// The reconciler may have created extra workspaces beyond the configured
	// target (e.g. replacements for failed builds), so using targetNumWorkspaces
	// as the denominator would undercount completed deletions.
	initialWorkspaces, err := r.client.Workspaces(deletionCtx, codersdk.WorkspaceFilter{
		Template: r.template.Name,
	})
	if err != nil {
		return xerrors.Errorf("list workspaces at deletion start: %w", err)
	}
	initialWorkspaceCount := len(initialWorkspaces.Workspaces)

	// retryCount tracks how many delete builds we've submitted per workspace.
	// lastRetriedBuildID prevents submitting a second retry for the same failed
	// build before the API reflects the new build.
	retryCount := make(map[uuid.UUID]int)
	lastRetriedBuildID := make(map[uuid.UUID]uuid.UUID)

	tkr := r.cfg.Clock.TickerFunc(deletionCtx, deletionPollInterval, func() error {
		workspaces, err := r.client.Workspaces(deletionCtx, codersdk.WorkspaceFilter{
			Template: r.template.Name,
		})
		if err != nil {
			return xerrors.Errorf("list workspaces: %w", err)
		}

		createdCount := 0
		runningCount := 0
		failedCount := 0
		exhaustedCount := 0

		for _, ws := range workspaces.Workspaces {
			if ws.LatestBuild.Transition != codersdk.WorkspaceTransitionDelete {
				// The reconciler hasn't submitted a delete build yet.
				continue
			}
			createdCount++

			switch ws.LatestBuild.Job.Status {
			case codersdk.ProvisionerJobRunning, codersdk.ProvisionerJobPending:
				runningCount++

			case codersdk.ProvisionerJobFailed, codersdk.ProvisionerJobCanceled:
				// Skip if we've already submitted a retry for this specific
				// failed build and are waiting for the new build to appear.
				if lastRetriedBuildID[ws.ID] == ws.LatestBuild.ID {
					runningCount++
					continue
				}

				if retryCount[ws.ID] >= maxDeletionRetries {
					exhaustedCount++
					failedCount++
					continue
				}

				retryCount[ws.ID]++
				lastRetriedBuildID[ws.ID] = ws.LatestBuild.ID
				logger.Warn(deletionCtx, "retrying failed workspace deletion",
					slog.F("workspace_id", ws.ID),
					slog.F("workspace_name", ws.Name),
					slog.F("attempt", retryCount[ws.ID]),
					slog.F("max_attempts", maxDeletionRetries),
				)
				_, retryErr := r.client.CreateWorkspaceBuild(deletionCtx, ws.ID, codersdk.CreateWorkspaceBuildRequest{
					Transition: codersdk.WorkspaceTransitionDelete,
				})
				if retryErr != nil {
					return xerrors.Errorf("retry workspace deletion (attempt %d): %w", retryCount[ws.ID], retryErr)
				}
				runningCount++
			}
		}

		completedCount := initialWorkspaceCount - len(workspaces.Workspaces)
		createdCount += completedCount

		r.cfg.Metrics.SetDeletionJobsCreated(createdCount, r.template.Name)
		r.cfg.Metrics.SetDeletionJobsRunning(runningCount, r.template.Name)
		r.cfg.Metrics.SetDeletionJobsFailed(failedCount, r.template.Name)
		r.cfg.Metrics.SetDeletionJobsCompleted(completedCount, r.template.Name)

		if len(workspaces.Workspaces) == 0 {
			return errTickerDone
		}

		// If every remaining workspace has exhausted all retries, fail
		// immediately rather than waiting for the timeout.
		if exhaustedCount > 0 && exhaustedCount == len(workspaces.Workspaces) {
			return xerrors.Errorf("%d workspace(s) failed to delete after %d attempts", exhaustedCount, maxDeletionRetries+1)
		}

		return nil
	}, "waitForPrebuildWorkspacesDeletion")
	err = tkr.Wait()
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
		TemplateID:      templateID,
		FileID:          uploadResp.ID,
		Message:         "Template version for scaletest prebuilds",
		StorageMethod:   codersdk.ProvisionerStorageMethodFile,
		Provisioner:     codersdk.ProvisionerTypeTerraform,
		ProvisionerTags: r.cfg.ProvisionerTags,
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

	logger.Info(ctx, "deleting template", slog.F("template_name", r.template.Name))

	err := r.client.DeleteTemplate(ctx, r.template.ID)
	if err != nil {
		return xerrors.Errorf("delete template: %w", err)
	}

	logger.Info(ctx, "template deleted successfully", slog.F("template_name", r.template.Name))
	return nil
}

//go:embed tf/main.tf.tpl
var templateContent string

func TemplateTarData(numPresets, numPresetPrebuilds int) ([]byte, error) {
	tmpl, err := template.New("prebuilds-template").Parse(templateContent)
	if err != nil {
		return nil, err
	}
	result := bytes.Buffer{}
	err = tmpl.Execute(&result, map[string]int{
		"NumPresets":         numPresets,
		"NumPresetPrebuilds": numPresetPrebuilds,
	})
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{
		"main.tf": result.Bytes(),
	}
	tarBytes, err := loadtestutil.CreateTarFromFiles(files)
	if err != nil {
		return nil, err
	}
	return tarBytes, nil
}
