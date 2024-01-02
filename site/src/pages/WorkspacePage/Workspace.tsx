import { type Interpolation, type Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import AlertTitle from "@mui/material/AlertTitle";
import { type FC, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import dayjs from "dayjs";
import type * as TypesGen from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { Margins } from "components/Margins/Margins";
import { Resources } from "components/Resources/Resources";
import { Stack } from "components/Stack/Stack";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { DormantWorkspaceBanner } from "components/WorkspaceDeletion";
import { AgentRow } from "components/Resources/AgentRow";
import { useLocalStorage } from "hooks";
import {
  ActiveTransition,
  WorkspaceBuildProgress,
} from "./WorkspaceBuildProgress";
import { BuildsTable } from "./BuildsTable";
import { WorkspaceDeletedBanner } from "./WorkspaceDeletedBanner";
import { WorkspaceTopbar } from "./WorkspaceTopbar/WorkspaceTopbar";

export type WorkspaceError =
  | "getBuildsError"
  | "buildError"
  | "cancellationError";

export type WorkspaceErrors = Partial<Record<WorkspaceError, unknown>>;

export interface WorkspaceProps {
  handleStart: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
  handleStop: () => void;
  handleRestart: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
  handleDelete: () => void;
  handleUpdate: () => void;
  handleCancel: () => void;
  handleSettings: () => void;
  handleChangeVersion: () => void;
  handleDormantActivate: () => void;
  isUpdating: boolean;
  isRestarting: boolean;
  workspace: TypesGen.Workspace;
  resources?: TypesGen.WorkspaceResource[];
  canUpdateWorkspace: boolean;
  updateMessage?: string;
  canChangeVersions: boolean;
  hideSSHButton?: boolean;
  hideVSCodeDesktopButton?: boolean;
  workspaceErrors: WorkspaceErrors;
  buildInfo?: TypesGen.BuildInfoResponse;
  sshPrefix?: string;
  template?: TypesGen.Template;
  canRetryDebugMode: boolean;
  handleBuildRetry: () => void;
  handleBuildRetryDebug: () => void;
  buildLogs?: React.ReactNode;
  builds: TypesGen.WorkspaceBuild[] | undefined;
  onLoadMoreBuilds: () => void;
  isLoadingMoreBuilds: boolean;
  hasMoreBuilds: boolean;
  canAutostart: boolean;
}

/**
 * Workspace is the top-level component for viewing an individual workspace
 */
export const Workspace: FC<React.PropsWithChildren<WorkspaceProps>> = ({
  handleStart,
  handleStop,
  handleRestart,
  handleDelete,
  handleUpdate,
  handleCancel,
  handleSettings,
  handleChangeVersion,
  handleDormantActivate,
  workspace,
  isUpdating,
  isRestarting,
  resources,
  builds,
  canUpdateWorkspace,
  updateMessage,
  canChangeVersions,
  workspaceErrors,
  hideSSHButton,
  hideVSCodeDesktopButton,
  buildInfo,
  sshPrefix,
  template,
  canRetryDebugMode,
  handleBuildRetry,
  handleBuildRetryDebug,
  buildLogs,
  onLoadMoreBuilds,
  isLoadingMoreBuilds,
  hasMoreBuilds,
  canAutostart,
}) => {
  const navigate = useNavigate();
  const { saveLocal, getLocal } = useLocalStorage();

  const [showAlertPendingInQueue, setShowAlertPendingInQueue] = useState(false);

  // 2023-11-15 - MES - This effect will be called every single render because
  // "now" will always change and invalidate the dependency array. Need to
  // figure out if this effect really should run every render (possibly meaning
  // no dependency array at all), or how to get the array stabilized (ideal)
  const now = dayjs();
  useEffect(() => {
    if (
      workspace.latest_build.status !== "pending" ||
      workspace.latest_build.job.queue_size === 0
    ) {
      if (!showAlertPendingInQueue) {
        return;
      }

      const hideTimer = setTimeout(() => {
        setShowAlertPendingInQueue(false);
      }, 250);
      return () => {
        clearTimeout(hideTimer);
      };
    }

    const t = Math.max(
      0,
      5000 - dayjs().diff(dayjs(workspace.latest_build.created_at)),
    );
    const showTimer = setTimeout(() => {
      setShowAlertPendingInQueue(true);
    }, t);

    return () => {
      clearTimeout(showTimer);
    };
  }, [workspace, now, showAlertPendingInQueue]);

  const updateRequired =
    (workspace.template_require_active_version ||
      workspace.automatic_updates === "always") &&
    workspace.outdated;
  const autoStartFailing = workspace.autostart_schedule && !canAutostart;
  const requiresManualUpdate = updateRequired && autoStartFailing;

  const transitionStats =
    template !== undefined ? ActiveTransition(template, workspace) : undefined;

  return (
    <>
      <WorkspaceTopbar
        workspace={workspace}
        handleStart={handleStart}
        handleStop={handleStop}
        handleRestart={handleRestart}
        handleDelete={handleDelete}
        handleUpdate={handleUpdate}
        handleCancel={handleCancel}
        handleSettings={handleSettings}
        handleBuildRetry={handleBuildRetry}
        handleBuildRetryDebug={handleBuildRetryDebug}
        handleChangeVersion={handleChangeVersion}
        handleDormantActivate={handleDormantActivate}
        canRetryDebugMode={canRetryDebugMode}
        canChangeVersions={canChangeVersions}
        isUpdating={isUpdating}
        isRestarting={isRestarting}
        canUpdateWorkspace={canUpdateWorkspace}
      />

      <Margins css={styles.content}>
        <Stack direction="column" css={styles.firstColumnSpacer} spacing={4}>
          {workspace.outdated &&
            (requiresManualUpdate ? (
              <Alert severity="warning">
                <AlertTitle>
                  Autostart has been disabled for your workspace.
                </AlertTitle>
                <AlertDetail>
                  Autostart is unable to automatically update your workspace.
                  Manually update your workspace to reenable Autostart.
                </AlertDetail>
              </Alert>
            ) : (
              <Alert severity="info">
                <AlertTitle>
                  An update is available for your workspace
                </AlertTitle>
                {updateMessage && <AlertDetail>{updateMessage}</AlertDetail>}
              </Alert>
            ))}

          {Boolean(workspaceErrors.buildError) && (
            <ErrorAlert error={workspaceErrors.buildError} dismissible />
          )}

          {Boolean(workspaceErrors.cancellationError) && (
            <ErrorAlert error={workspaceErrors.cancellationError} dismissible />
          )}

          {workspace.latest_build.status === "running" &&
            !workspace.health.healthy && (
              <Alert
                severity="warning"
                actions={
                  canUpdateWorkspace && (
                    <Button
                      variant="text"
                      size="small"
                      onClick={() => {
                        handleRestart();
                      }}
                    >
                      Restart
                    </Button>
                  )
                }
              >
                <AlertTitle>Workspace is unhealthy</AlertTitle>
                <AlertDetail>
                  Your workspace is running but{" "}
                  {workspace.health.failing_agents.length > 1
                    ? `${workspace.health.failing_agents.length} agents are unhealthy`
                    : `1 agent is unhealthy`}
                  .
                </AlertDetail>
              </Alert>
            )}

          {workspace.latest_build.status === "deleted" && (
            <WorkspaceDeletedBanner
              handleClick={() => navigate(`/templates`)}
            />
          )}
          {/* <DormantWorkspaceBanner/> determines its own visibility */}
          <DormantWorkspaceBanner
            workspace={workspace}
            shouldRedisplayBanner={
              getLocal("dismissedWorkspace") !== workspace.id
            }
            onDismiss={() => saveLocal("dismissedWorkspace", workspace.id)}
          />

          {showAlertPendingInQueue && (
            <Alert severity="info">
              <AlertTitle>Workspace build is pending</AlertTitle>
              <AlertDetail>
                <div css={styles.alertPendingInQueue}>
                  This workspace build job is waiting for a provisioner to
                  become available. If you have been waiting for an extended
                  period of time, please contact your administrator for
                  assistance.
                </div>
                <div>
                  Position in queue:{" "}
                  <strong>{workspace.latest_build.job.queue_position}</strong>
                </div>
              </AlertDetail>
            </Alert>
          )}

          {workspace.latest_build.job.error && (
            <Alert
              severity="error"
              actions={
                <Button
                  onClick={
                    canRetryDebugMode ? handleBuildRetryDebug : handleBuildRetry
                  }
                  variant="text"
                  size="small"
                >
                  Retry{canRetryDebugMode && " in debug mode"}
                </Button>
              }
            >
              <AlertTitle>Workspace build failed</AlertTitle>
              <AlertDetail>{workspace.latest_build.job.error}</AlertDetail>
            </Alert>
          )}

          {template?.deprecated && (
            <Alert severity="warning">
              <AlertTitle>Workspace using deprecated template</AlertTitle>
              <AlertDetail>{template?.deprecation_message}</AlertDetail>
            </Alert>
          )}

          {transitionStats !== undefined && (
            <WorkspaceBuildProgress
              workspace={workspace}
              transitionStats={transitionStats}
            />
          )}

          {buildLogs}

          {typeof resources !== "undefined" && resources.length > 0 && (
            <Resources
              resources={resources}
              agentRow={(agent) => (
                <AgentRow
                  key={agent.id}
                  agent={agent}
                  workspace={workspace}
                  sshPrefix={sshPrefix}
                  showApps={canUpdateWorkspace}
                  showBuiltinApps={canUpdateWorkspace}
                  hideSSHButton={hideSSHButton}
                  hideVSCodeDesktopButton={hideVSCodeDesktopButton}
                  serverVersion={buildInfo?.version || ""}
                  serverAPIVersion={buildInfo?.agent_api_version || ""}
                  onUpdateAgent={handleUpdate} // On updating the workspace the agent version is also updated
                />
              )}
            />
          )}

          {workspaceErrors.getBuildsError ? (
            <ErrorAlert error={workspaceErrors.getBuildsError} />
          ) : (
            <BuildsTable
              builds={builds}
              onLoadMoreBuilds={onLoadMoreBuilds}
              isLoadingMoreBuilds={isLoadingMoreBuilds}
              hasMoreBuilds={hasMoreBuilds}
            />
          )}
        </Stack>
      </Margins>
    </>
  );
};

const styles = {
  content: {
    marginTop: 32,
  },

  actions: (theme) => ({
    [theme.breakpoints.down("md")]: {
      flexDirection: "column",
    },
  }),

  firstColumnSpacer: {
    flex: 2,
  },

  alertPendingInQueue: {
    marginBottom: 12,
  },
} satisfies Record<string, Interpolation<Theme>>;
