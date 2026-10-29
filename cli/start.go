package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) start() *serpent.Command {
	var (
		parameterFlags workspaceParameterFlags
		bflags         buildFlags

		noWait          bool
		templateVersion string
	)

	cmd := &serpent.Command{
		Annotations: serpent.Annotations(workspaceCommand).Mark(annotationClientSessionID, "").Mark(annotationFlightRecorder, ""),
		Use:         "start <workspace>",
		Short:       "Start a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Flag:        "no-wait",
				Description: "Return immediately after starting the workspace.",
				Value:       serpent.BoolOf(&noWait),
				Hidden:      false,
			},
			{
				Flag:        "template-version",
				Description: "Start with a version of the workspace template, by name or ID.",
				Value:       serpent.StringOf(&templateVersion),
			},
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			// The invocation logger records debug detail and emits it to stderr
			// only if the command fails (see flightRecorderMiddleware).
			client.SetLogger(inv.Logger)

			workspace, err := client.ResolveWorkspace(inv.Context(), inv.Args[0])
			if err != nil {
				return err
			}
			versionID, err := resolveTemplateVersionID(inv.Context(), client, workspace.TemplateID, workspace.TemplateName, templateVersion)
			if err != nil {
				return err
			}
			if versionID != uuid.Nil && (workspace.LatestBuild.Status == codersdk.WorkspaceStatusPending || workspace.LatestBuild.Status == codersdk.WorkspaceStatusStarting) {
				return xerrors.Errorf("cannot start with a template version while the workspace is %s; wait for the current build to finish", workspace.LatestBuild.Status)
			}
			var build codersdk.WorkspaceBuild
			switch {
			case workspace.LatestBuild.Status == codersdk.WorkspaceStatusPending:
				// The above check is technically duplicated in cliutil.WarnmatchedProvisioners
				// but we still want to avoid users spamming multiple builds that will
				// not be picked up.
				_, _ = fmt.Fprintf(
					inv.Stdout,
					"\nThe %s workspace is waiting to start!\n",
					cliui.Keyword(workspace.Name),
				)
				cliutil.WarnMatchedProvisioners(inv.Stderr, workspace.LatestBuild.MatchedProvisioners, workspace.LatestBuild.Job)
				if _, err := cliui.Prompt(inv, cliui.PromptOptions{
					Text:      "Enqueue another start?",
					IsConfirm: true,
					Default:   cliui.ConfirmNo,
				}); err != nil {
					return err
				}
			case workspace.LatestBuild.Status == codersdk.WorkspaceStatusRunning && versionID == uuid.Nil:
				_, _ = fmt.Fprintf(
					inv.Stdout, "\nThe %s workspace is already running!\n",
					cliui.Keyword(workspace.Name),
				)
				return nil
			case workspace.LatestBuild.Status == codersdk.WorkspaceStatusStarting:
				_, _ = fmt.Fprintf(
					inv.Stdout, "\nThe %s workspace is already starting.\n",
					cliui.Keyword(workspace.Name),
				)
				build = workspace.LatestBuild
			default:
				// If the last build was a failed start, run a stop
				// first to clean up any partially-provisioned
				// resources.
				if workspace.LatestBuild.Status == codersdk.WorkspaceStatusFailed &&
					workspace.LatestBuild.Transition == codersdk.WorkspaceTransitionStart {
					_, _ = fmt.Fprintf(inv.Stdout, "The last start build failed. Cleaning up before retrying...\n")
					stopBuild, stopErr := client.CreateWorkspaceBuild(inv.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
						Transition: codersdk.WorkspaceTransitionStop,
					})
					if stopErr != nil {
						return xerrors.Errorf("cleanup stop after failed start: %w", stopErr)
					}
					stopErr = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, stopBuild.ID)
					if stopErr != nil {
						return xerrors.Errorf("wait for cleanup stop: %w", stopErr)
					}
					// Re-fetch workspace after stop completes so
					// startWorkspace sees the latest state.
					workspace, err = client.ResolveWorkspace(inv.Context(), inv.Args[0])
					if err != nil {
						return err
					}
				}
				build, err = startWorkspace(inv, client, workspace, parameterFlags, bflags, WorkspaceStart, versionID)
				// It's possible for a workspace build to fail due to the template requiring starting
				// workspaces with the active version.
				if cerr, ok := codersdk.AsError(err); ok && versionID == uuid.Nil && cerr.StatusCode() == http.StatusForbidden {
					_, _ = fmt.Fprintln(inv.Stdout, "Unable to start the workspace with the template version from the last build. Policy may require you to restart with the current active template version.")
					build, err = startWorkspace(inv, client, workspace, parameterFlags, bflags, WorkspaceUpdate, uuid.Nil)
					if err != nil {
						return xerrors.Errorf("start workspace with active template version: %w", err)
					}
				} else if err != nil {
					return err
				}
			}

			if noWait {
				_, _ = fmt.Fprintf(inv.Stdout, "The %s workspace has been started in no-wait mode. Workspace is building in the background.\n", cliui.Keyword(workspace.Name))
				return nil
			}

			err = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, build.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(
				inv.Stdout, "\nThe %s workspace has been started at %s!\n",
				cliui.Keyword(workspace.Name), cliui.Timestamp(time.Now()),
			)
			return nil
		},
	}

	cmd.Options = append(cmd.Options, parameterFlags.allOptions()...)
	cmd.Options = append(cmd.Options, bflags.cliOptions()...)

	return cmd
}

func buildWorkspaceStartRequest(inv *serpent.Invocation, client *codersdk.Client, workspace codersdk.Workspace, parameterFlags workspaceParameterFlags, buildFlags buildFlags, action WorkspaceCLIAction, templateVersionID uuid.UUID) (codersdk.CreateWorkspaceBuildRequest, error) {
	version := workspace.LatestBuild.TemplateVersionID

	if templateVersionID != uuid.Nil {
		version = templateVersionID
	} else if workspace.AutomaticUpdates == codersdk.AutomaticUpdatesAlways || workspace.TemplateRequireActiveVersion || action == WorkspaceUpdate {
		version = workspace.TemplateActiveVersionID
	}
	if version != workspace.LatestBuild.TemplateVersionID {
		action = WorkspaceUpdate
	}

	lastBuildParameters, err := client.WorkspaceBuildParameters(inv.Context(), workspace.LatestBuild.ID)
	if err != nil {
		return codersdk.CreateWorkspaceBuildRequest{}, err
	}

	ephemeralParameters, err := asWorkspaceBuildParameters(parameterFlags.ephemeralParameters)
	if err != nil {
		return codersdk.CreateWorkspaceBuildRequest{}, xerrors.Errorf("unable to parse build options: %w", err)
	}

	cliRichParameters, err := asWorkspaceBuildParameters(parameterFlags.richParameters)
	if err != nil {
		return codersdk.CreateWorkspaceBuildRequest{}, xerrors.Errorf("unable to parse rich parameters: %w", err)
	}

	cliRichParameterDefaults, err := asWorkspaceBuildParameters(parameterFlags.richParameterDefaults)
	if err != nil {
		return codersdk.CreateWorkspaceBuildRequest{}, xerrors.Errorf("unable to parse rich parameter defaults: %w", err)
	}

	buildParameters, err := prepWorkspaceBuild(inv, client, prepWorkspaceBuildArgs{
		Action:              action,
		TemplateVersionID:   version,
		NewWorkspaceName:    workspace.Name,
		LastBuildParameters: lastBuildParameters,
		Owner:               workspace.OwnerID.String(),

		PromptEphemeralParameters: parameterFlags.promptEphemeralParameters,
		EphemeralParameters:       ephemeralParameters,
		PromptRichParameters:      parameterFlags.promptRichParameters,
		RichParameters:            cliRichParameters,
		RichParameterFile:         parameterFlags.richParameterFile,
		RichParameterDefaults:     cliRichParameterDefaults,
		UseParameterDefaults:      parameterFlags.useParameterDefaults,
	})
	if err != nil {
		return codersdk.CreateWorkspaceBuildRequest{}, err
	}

	wbr := codersdk.CreateWorkspaceBuildRequest{
		Transition:          codersdk.WorkspaceTransitionStart,
		RichParameterValues: buildParameters,
		TemplateVersionID:   version,
	}
	if buildFlags.provisionerLogDebug {
		wbr.LogLevel = codersdk.ProvisionerLogLevelDebug
	}
	if buildFlags.reason != "" {
		wbr.Reason = codersdk.CreateWorkspaceBuildReason(buildFlags.reason)
	}

	return wbr, nil
}

func startWorkspace(inv *serpent.Invocation, client *codersdk.Client, workspace codersdk.Workspace, parameterFlags workspaceParameterFlags, buildFlags buildFlags, action WorkspaceCLIAction, templateVersionID uuid.UUID) (codersdk.WorkspaceBuild, error) {
	if workspace.DormantAt != nil {
		_, _ = fmt.Fprintln(inv.Stdout, "Activating dormant workspace...")
		err := client.UpdateWorkspaceDormancy(inv.Context(), workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: false,
		})
		if err != nil {
			return codersdk.WorkspaceBuild{}, xerrors.Errorf("activate workspace: %w", err)
		}
	}
	req, err := buildWorkspaceStartRequest(inv, client, workspace, parameterFlags, buildFlags, action, templateVersionID)
	if err != nil {
		return codersdk.WorkspaceBuild{}, err
	}

	build, err := client.CreateWorkspaceBuild(inv.Context(), workspace.ID, req)
	if err != nil {
		return codersdk.WorkspaceBuild{}, xerrors.Errorf("create workspace build: %w", err)
	}
	cliutil.WarnMatchedProvisioners(inv.Stderr, build.MatchedProvisioners, build.Job)

	return build, nil
}

// resolveTemplateVersionID looks up a template version of the given template
// by name or ID. Names take precedence, so a version named like a UUID is
// matched by name. An empty input returns uuid.Nil so callers can fall back to
// their default version.
func resolveTemplateVersionID(ctx context.Context, client *codersdk.Client, templateID uuid.UUID, templateName, nameOrID string) (uuid.UUID, error) {
	if nameOrID == "" {
		return uuid.Nil, nil
	}
	notFound := xerrors.Errorf("template version %q not found for template %q", nameOrID, templateName)
	if nameOrID == "." || nameOrID == ".." {
		return uuid.Nil, notFound
	}
	// Escaping keeps the input from adding path segments or a query.
	// "." and ".." are rejected above.
	version, err := client.TemplateVersionByName(ctx, templateID, url.PathEscape(nameOrID))
	if err != nil {
		if !isNotFoundError(err) {
			return uuid.Nil, xerrors.Errorf("get template version by name: %w", err)
		}
		id, parseErr := uuid.Parse(nameOrID)
		if parseErr != nil {
			return uuid.Nil, notFound
		}
		version, err = client.TemplateVersion(ctx, id)
		if err != nil {
			if isNotFoundError(err) {
				return uuid.Nil, notFound
			}
			return uuid.Nil, xerrors.Errorf("get template version by ID: %w", err)
		}
	}
	// The ID lookup can return any template's version, so check ownership
	// for it and for anything the name route returns.
	if version.TemplateID == nil || *version.TemplateID != templateID {
		return uuid.Nil, xerrors.Errorf("template version %q does not belong to template %q", nameOrID, templateName)
	}
	return version.ID, nil
}

func isNotFoundError(err error) bool {
	sdkErr, ok := codersdk.AsError(err)
	return ok && sdkErr.StatusCode() == http.StatusNotFound
}
