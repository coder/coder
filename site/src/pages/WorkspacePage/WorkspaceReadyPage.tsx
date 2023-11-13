import { useDashboard } from "components/Dashboard/DashboardProvider";
import dayjs from "dayjs";
import { useFeatureVisibility } from "hooks/useFeatureVisibility";
import { FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate } from "react-router-dom";
import {
  getDeadline,
  getMaxDeadline,
  getMaxDeadlineChange,
  getMinDeadline,
} from "utils/schedule";
import { StateFrom } from "xstate";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { Workspace, WorkspaceErrors } from "./Workspace";
import { pageTitle } from "utils/page";
import { hasJobError } from "utils/workspace";
import {
  WorkspaceEvent,
  workspaceMachine,
} from "xServices/workspace/workspaceXService";
import { UpdateBuildParametersDialog } from "./UpdateBuildParametersDialog";
import { ChangeVersionDialog } from "./ChangeVersionDialog";
import { useMutation, useQuery } from "react-query";
import { restartWorkspace } from "api/api";
import {
  ConfirmDialog,
  ConfirmDialogProps,
} from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import * as TypesGen from "api/typesGenerated";
import { WorkspaceBuildLogsSection } from "./WorkspaceBuildLogsSection";
import { templateVersion, templateVersions } from "api/queries/templates";
import { Alert } from "components/Alert/Alert";
import { Stack } from "components/Stack/Stack";
import { useWorkspaceBuildLogs } from "hooks/useWorkspaceBuildLogs";
import { decreaseDeadline, increaseDeadline } from "api/queries/workspaces";
import { getErrorMessage } from "api/errors";
import { displaySuccess, displayError } from "components/GlobalSnackbar/utils";
import { deploymentConfig, deploymentSSHConfig } from "api/queries/deployment";
import { WorkspacePermissions } from "./permissions";
import { workspaceResolveAutostart } from "api/queries/workspaceQuota";

interface WorkspaceReadyPageProps {
  template: TypesGen.Template;
  workspace: TypesGen.Workspace;
  permissions: WorkspacePermissions;
  workspaceState: Omit<
    StateFrom<typeof workspaceMachine>,
    "template" | "workspace" | "permissions"
  >;
  workspaceSend: (event: WorkspaceEvent) => void;
  builds: TypesGen.WorkspaceBuild[] | undefined;
  buildsError: unknown;
  onLoadMoreBuilds: () => void;
  isLoadingMoreBuilds: boolean;
  hasMoreBuilds: boolean;
}

export const WorkspaceReadyPage = ({
  workspace,
  template,
  permissions,
  workspaceState,
  workspaceSend,
  builds,
  buildsError,
  onLoadMoreBuilds,
  isLoadingMoreBuilds,
  hasMoreBuilds,
}: WorkspaceReadyPageProps): JSX.Element => {
  const navigate = useNavigate();
  const { buildInfo } = useDashboard();
  const featureVisibility = useFeatureVisibility();
  const { buildError, cancellationError, missedParameters } =
    workspaceState.context;
  if (workspace === undefined) {
    throw Error("Workspace is undefined");
  }
  const canUpdateWorkspace = Boolean(permissions?.updateWorkspace);
  const canUpdateTemplate = Boolean(permissions?.updateTemplate);
  const { data: deploymentValues } = useQuery({
    ...deploymentConfig(),
    enabled: permissions?.viewDeploymentValues,
  });
  const canRetryDebugMode = Boolean(
    deploymentValues?.config.enable_terraform_debug_mode,
  );
  const [changeVersionDialogOpen, setChangeVersionDialogOpen] = useState(false);
  const [isConfirmingUpdate, setIsConfirmingUpdate] = useState(false);

  // Versions
  const { data: allVersions } = useQuery({
    ...templateVersions(workspace.template_id),
    enabled: changeVersionDialogOpen,
  });
  const { data: latestVersion } = useQuery({
    ...templateVersion(workspace.template_active_version_id),
    enabled: workspace.outdated,
  });

  // Build logs
  const buildLogs = useWorkspaceBuildLogs(workspace.latest_build.id);
  const shouldDisplayBuildLogs =
    hasJobError(workspace) ||
    ["canceling", "deleting", "pending", "starting", "stopping"].includes(
      workspace.latest_build.status,
    );

  // Restart
  const [confirmingRestart, setConfirmingRestart] = useState<{
    open: boolean;
    buildParameters?: TypesGen.WorkspaceBuildParameter[];
  }>({ open: false });
  const {
    mutate: mutateRestartWorkspace,
    error: restartBuildError,
    isLoading: isRestarting,
  } = useMutation({
    mutationFn: restartWorkspace,
  });

  // Schedule controls
  const deadline = getDeadline(workspace);
  const onDeadlineChangeSuccess = () => {
    displaySuccess("Updated workspace shutdown time.");
  };
  const onDeadlineChangeFails = (error: unknown) => {
    displayError(
      getErrorMessage(error, "Failed to update workspace shutdown time."),
    );
  };
  const decreaseMutation = useMutation({
    ...decreaseDeadline(workspace),
    onSuccess: onDeadlineChangeSuccess,
    onError: onDeadlineChangeFails,
  });
  const increaseMutation = useMutation({
    ...increaseDeadline(workspace),
    onSuccess: onDeadlineChangeSuccess,
    onError: onDeadlineChangeFails,
  });

  // Auto start
  const canAutostartResponse = useQuery(
    workspaceResolveAutostart(workspace.id),
  );
  const canAutostart = !canAutostartResponse.data?.parameter_mismatch ?? false;

  // SSH Prefix
  const sshPrefixQuery = useQuery(deploymentSSHConfig());

  // Favicon
  const favicon = getFaviconByStatus(workspace.latest_build);
  const [faviconTheme, setFaviconTheme] = useState<"light" | "dark">("dark");
  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) {
      return;
    }

    const isDark = window.matchMedia("(prefers-color-scheme: dark)");
    // We want the favicon the opposite of the theme.
    setFaviconTheme(isDark.matches ? "light" : "dark");
  }, []);

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${workspace.owner_name}/${workspace.name}`)}</title>
        <link
          rel="alternate icon"
          type="image/png"
          href={`/favicons/${favicon}-${faviconTheme}.png`}
        />
        <link
          rel="icon"
          type="image/svg+xml"
          href={`/favicons/${favicon}-${faviconTheme}.svg`}
        />
      </Helmet>

      <Workspace
        scheduleProps={{
          onDeadlineMinus: decreaseMutation.mutate,
          onDeadlinePlus: increaseMutation.mutate,
          maxDeadlineDecrease: getMaxDeadlineChange(deadline, getMinDeadline()),
          maxDeadlineIncrease: getMaxDeadlineChange(
            getMaxDeadline(workspace),
            deadline,
          ),
        }}
        isUpdating={workspaceState.matches("ready.build.requestingUpdate")}
        isRestarting={isRestarting}
        workspace={workspace}
        handleStart={(buildParameters) =>
          workspaceSend({ type: "START", buildParameters })
        }
        handleStop={() => workspaceSend({ type: "STOP" })}
        handleDelete={() => workspaceSend({ type: "ASK_DELETE" })}
        handleRestart={(buildParameters) => {
          setConfirmingRestart({ open: true, buildParameters });
        }}
        handleUpdate={() => {
          setIsConfirmingUpdate(true);
        }}
        handleCancel={() => workspaceSend({ type: "CANCEL" })}
        handleSettings={() => navigate("settings")}
        handleBuildRetry={() => workspaceSend({ type: "RETRY_BUILD" })}
        handleChangeVersion={() => {
          setChangeVersionDialogOpen(true);
        }}
        handleDormantActivate={() => workspaceSend({ type: "ACTIVATE" })}
        resources={workspace.latest_build.resources}
        builds={builds}
        onLoadMoreBuilds={onLoadMoreBuilds}
        isLoadingMoreBuilds={isLoadingMoreBuilds}
        hasMoreBuilds={hasMoreBuilds}
        canUpdateWorkspace={canUpdateWorkspace}
        updateMessage={latestVersion?.message}
        canRetryDebugMode={canRetryDebugMode}
        canChangeVersions={canUpdateTemplate}
        hideSSHButton={featureVisibility["browser_only"]}
        hideVSCodeDesktopButton={featureVisibility["browser_only"]}
        workspaceErrors={{
          [WorkspaceErrors.GET_BUILDS_ERROR]: buildsError,
          [WorkspaceErrors.BUILD_ERROR]: buildError || restartBuildError,
          [WorkspaceErrors.CANCELLATION_ERROR]: cancellationError,
        }}
        buildInfo={buildInfo}
        sshPrefix={sshPrefixQuery.data?.hostname_prefix}
        template={template}
        buildLogs={
          shouldDisplayBuildLogs && (
            <WorkspaceBuildLogsSection logs={buildLogs} />
          )
        }
        canAutostart={canAutostart}
      />
      <DeleteDialog
        entity="workspace"
        name={workspace.name}
        info={`This workspace was created ${dayjs(
          workspace.created_at,
        ).fromNow()}.`}
        isOpen={workspaceState.matches({ ready: { build: "askingDelete" } })}
        onCancel={() => workspaceSend({ type: "CANCEL_DELETE" })}
        onConfirm={() => {
          workspaceSend({ type: "DELETE" });
        }}
      />
      <UpdateBuildParametersDialog
        missedParameters={missedParameters ?? []}
        open={workspaceState.matches(
          "ready.build.askingForMissedBuildParameters",
        )}
        onClose={() => {
          workspaceSend({ type: "CANCEL" });
        }}
        onUpdate={(buildParameters) => {
          workspaceSend({ type: "UPDATE", buildParameters });
        }}
      />
      <ChangeVersionDialog
        templateVersions={allVersions?.reverse()}
        template={template}
        defaultTemplateVersion={allVersions?.find(
          (v) => workspace.latest_build.template_version_id === v.id,
        )}
        open={changeVersionDialogOpen}
        onClose={() => {
          setChangeVersionDialogOpen(false);
        }}
        onConfirm={(templateVersion) => {
          setChangeVersionDialogOpen(false);
          workspaceSend({
            type: "CHANGE_VERSION",
            templateVersionId: templateVersion.id,
          });
        }}
      />
      <WarningDialog
        open={isConfirmingUpdate}
        onConfirm={() => {
          workspaceSend({ type: "UPDATE" });
          setIsConfirmingUpdate(false);
        }}
        onClose={() => setIsConfirmingUpdate(false)}
        title="Update and restart?"
        confirmText="Update"
        description={
          <Stack>
            <p>
              Restarting your workspace will stop all running processes and{" "}
              <strong>delete non-persistent data</strong>.
            </p>
            {latestVersion?.message && (
              <Alert severity="info">{latestVersion.message}</Alert>
            )}
          </Stack>
        }
      />

      <WarningDialog
        open={confirmingRestart.open}
        onConfirm={() => {
          mutateRestartWorkspace({
            workspace,
            buildParameters: confirmingRestart.buildParameters,
          });
          setConfirmingRestart({ open: false });
        }}
        onClose={() => setConfirmingRestart({ open: false })}
        title="Restart your workspace?"
        confirmText="Restart"
        description={
          <>
            Restarting your workspace will stop all running processes and{" "}
            <strong>delete non-persistent data</strong>.
          </>
        }
      />
    </>
  );
};

const WarningDialog: FC<
  Pick<
    ConfirmDialogProps,
    "open" | "onClose" | "title" | "confirmText" | "description" | "onConfirm"
  >
> = (props) => {
  return <ConfirmDialog type="info" hideCancel={false} {...props} />;
};

// You can see the favicon designs here: https://www.figma.com/file/YIGBkXUcnRGz2ZKNmLaJQf/Coder-v2-Design?node-id=560%3A620
type FaviconType =
  | "favicon"
  | "favicon-success"
  | "favicon-error"
  | "favicon-warning"
  | "favicon-running";

const getFaviconByStatus = (build: TypesGen.WorkspaceBuild): FaviconType => {
  switch (build.status) {
    case undefined:
      return "favicon";
    case "running":
      return "favicon-success";
    case "starting":
      return "favicon-running";
    case "stopping":
      return "favicon-running";
    case "stopped":
      return "favicon";
    case "deleting":
      return "favicon";
    case "deleted":
      return "favicon";
    case "canceling":
      return "favicon-warning";
    case "canceled":
      return "favicon";
    case "failed":
      return "favicon-error";
    case "pending":
      return "favicon";
  }
};
