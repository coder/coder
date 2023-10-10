import { useActor } from "@xstate/react";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import dayjs from "dayjs";
import { useFeatureVisibility } from "hooks/useFeatureVisibility";
import { type FC, useEffect, useState } from "react";
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
import { Workspace } from "./Workspace";
import { pageTitle } from "utils/page";
import { getFaviconByStatus, hasJobError } from "utils/workspace";
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
import { type CreateWorkspaceMode } from "xServices/createWorkspace/createWorkspaceXService";

interface WorkspaceReadyPageProps {
  workspaceState: StateFrom<typeof workspaceMachine>;
  workspaceSend: (event: WorkspaceEvent) => void;
  quota?: TypesGen.WorkspaceQuota;
  builds: TypesGen.WorkspaceBuild[] | undefined;
  buildsError: unknown;
  onLoadMoreBuilds: () => void;
  isLoadingMoreBuilds: boolean;
  hasMoreBuilds: boolean;
}

export const WorkspaceReadyPage = ({
  workspaceState,
  workspaceSend,
  quota,
  builds,
  buildsError,
  onLoadMoreBuilds,
  isLoadingMoreBuilds,
  hasMoreBuilds,
}: WorkspaceReadyPageProps): JSX.Element => {
  const {
    workspace,
    template,
    templateVersion: currentVersion,
    deploymentValues,
    buildError,
    cancellationError,
    sshPrefix,
    permissions,
    missedParameters,
  } = workspaceState.context;

  // Breaks the rules of hooks, but if we're throwing an error, the rules won't
  // even have a chance to matter. Best to get the check out of the way early
  // for better type narrowing
  if (workspace === undefined) {
    throw Error("Workspace is undefined");
  }

  const { buildInfo } = useDashboard();
  const featureVisibility = useFeatureVisibility();
  const navigate = useNavigate();

  const [_, bannerSend] = useActor(
    workspaceState.children["scheduleBannerMachine"],
  );

  const [changeVersionDialogOpen, setChangeVersionDialogOpen] = useState(false);
  const [isConfirmingUpdate, setIsConfirmingUpdate] = useState(false);
  const [confirmingRestart, setConfirmingRestart] = useState<{
    open: boolean;
    buildParameters?: TypesGen.WorkspaceBuildParameter[];
  }>({ open: false });

  const { data: allTemplateVersions } = useQuery({
    ...templateVersions(workspace.template_id),
    enabled: changeVersionDialogOpen,
  });

  const { data: latestTemplateVersion } = useQuery({
    ...templateVersion(workspace.template_active_version_id),
    enabled: workspace.outdated,
  });

  const buildLogs = useWorkspaceBuildLogs(workspace.latest_build.id);
  const faviconTheme = useFaviconTheme();

  const {
    mutate: mutateRestartWorkspace,
    error: restartBuildError,
    isLoading: isRestarting,
  } = useMutation({
    mutationFn: restartWorkspace,
  });

  // keep banner machine in sync with workspace
  useEffect(() => {
    bannerSend({ type: "REFRESH_WORKSPACE", workspace });
  }, [bannerSend, workspace]);

  const handleWorkspaceCloning = () => {
    if (template?.name === undefined) {
      return;
    }

    const workspaceCreationParams = new URLSearchParams({
      mode: "duplicate" satisfies CreateWorkspaceMode,
      name: workspace.name,
    });

    navigate({
      pathname: `/templates/${template.name}/workspace`,
      search: workspaceCreationParams.toString(),
    });
  };

  const favicon = getFaviconByStatus(workspace.latest_build);
  const deadline = getDeadline(workspace);
  const canUpdateWorkspace = Boolean(permissions?.updateWorkspace);
  const canUpdateTemplate = Boolean(permissions?.updateTemplate);
  const canRetryDebugMode =
    Boolean(permissions?.viewDeploymentValues) &&
    Boolean(deploymentValues?.enable_terraform_debug_mode);

  const shouldDisplayBuildLogs =
    hasJobError(workspace) ||
    ["canceling", "deleting", "pending", "starting", "stopping"].includes(
      workspace.latest_build.status,
    );

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
          onDeadlineMinus: (hours: number) => {
            bannerSend({
              type: "DECREASE_DEADLINE",
              hours,
            });
          },
          onDeadlinePlus: (hours: number) => {
            bannerSend({
              type: "INCREASE_DEADLINE",
              hours,
            });
          },
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
        handleClone={handleWorkspaceCloning}
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
        updateMessage={latestTemplateVersion?.message}
        canRetryDebugMode={canRetryDebugMode}
        canChangeVersions={canUpdateTemplate}
        hideSSHButton={featureVisibility["browser_only"]}
        hideVSCodeDesktopButton={featureVisibility["browser_only"]}
        workspaceErrors={{
          getBuildsError: buildsError,
          buildError: buildError || restartBuildError,
          cancellationError: cancellationError,
        }}
        buildInfo={buildInfo}
        sshPrefix={sshPrefix}
        template={template}
        quotaBudget={quota?.budget}
        templateWarnings={currentVersion?.warnings}
        buildLogs={
          shouldDisplayBuildLogs && (
            <WorkspaceBuildLogsSection logs={buildLogs} />
          )
        }
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
        templateVersions={allTemplateVersions?.reverse()}
        template={template}
        defaultTemplateVersion={allTemplateVersions?.find(
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
            {latestTemplateVersion?.message && (
              <Alert severity="info">{latestTemplateVersion.message}</Alert>
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

function useFaviconTheme() {
  const [faviconTheme, setFaviconTheme] = useState<"light" | "dark">("dark");

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) {
      return;
    }
    const isDark = window.matchMedia("(prefers-color-scheme: dark)");
    // We want the favicon the opposite of the theme.
    setFaviconTheme(isDark ? "light" : "dark");
  }, []);

  return faviconTheme;
}

const WarningDialog: FC<
  Pick<
    ConfirmDialogProps,
    "open" | "onClose" | "title" | "confirmText" | "description" | "onConfirm"
  >
> = (props) => {
  return <ConfirmDialog type="info" hideCancel={false} {...props} />;
};
