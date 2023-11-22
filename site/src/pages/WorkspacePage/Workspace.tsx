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
import {
  FullWidthPageHeader,
  PageHeaderActions,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/FullWidthPageHeader";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { DormantWorkspaceBanner } from "components/WorkspaceDeletion";
import { Avatar } from "components/Avatar/Avatar";
import { AgentRow } from "components/Resources/AgentRow";
import { useLocalStorage } from "hooks";
import { WorkspaceActions } from "pages/WorkspacePage/WorkspaceActions/WorkspaceActions";
import {
  ActiveTransition,
  WorkspaceBuildProgress,
} from "./WorkspaceBuildProgress";
import { BuildsTable } from "./BuildsTable";
import { WorkspaceDeletedBanner } from "./WorkspaceDeletedBanner";
import { WorkspaceStats } from "./WorkspaceStats";

export enum WorkspaceErrors {
  GET_BUILDS_ERROR = "getBuildsError",
  BUILD_ERROR = "buildError",
  CANCELLATION_ERROR = "cancellationError",
}
export interface WorkspaceProps {
  scheduleProps: {
    onDeadlinePlus: (hours: number) => void;
    onDeadlineMinus: (hours: number) => void;
    maxDeadlineIncrease: number;
    maxDeadlineDecrease: number;
  };
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
  canRetryDebugMode: boolean;
  canChangeVersions: boolean;
  hideSSHButton?: boolean;
  hideVSCodeDesktopButton?: boolean;
  workspaceErrors: Partial<Record<WorkspaceErrors, unknown>>;
  buildInfo?: TypesGen.BuildInfoResponse;
  sshPrefix?: string;
  template?: TypesGen.Template;
  quotaBudget?: number;
  handleBuildRetry: () => void;
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
  scheduleProps,
  handleStart,
  handleStop,
  handleRestart,
  handleDelete,
  handleUpdate,
  handleCancel,
  handleSettings,
  handleChangeVersion,
  handleDormantActivate: handleDormantActivate,
  workspace,
  isUpdating,
  isRestarting,
  resources,
  builds,
  canUpdateWorkspace,
  updateMessage,
  canRetryDebugMode,
  canChangeVersions,
  workspaceErrors,
  hideSSHButton,
  hideVSCodeDesktopButton,
  buildInfo,
  sshPrefix,
  template,
  handleBuildRetry,
  buildLogs,
  onLoadMoreBuilds,
  isLoadingMoreBuilds,
  hasMoreBuilds,
  canAutostart,
}) => {
  const navigate = useNavigate();
  const serverVersion = buildInfo?.version || "";
  const { saveLocal, getLocal } = useLocalStorage();

  const buildError = Boolean(workspaceErrors[WorkspaceErrors.BUILD_ERROR]) && (
    <ErrorAlert
      error={workspaceErrors[WorkspaceErrors.BUILD_ERROR]}
      dismissible
    />
  );

  const cancellationError = Boolean(
    workspaceErrors[WorkspaceErrors.CANCELLATION_ERROR],
  ) && (
    <ErrorAlert
      error={workspaceErrors[WorkspaceErrors.CANCELLATION_ERROR]}
      dismissible
    />
  );

  let transitionStats: TypesGen.TransitionStats | undefined = undefined;
  if (template !== undefined) {
    transitionStats = ActiveTransition(template, workspace);
  }

  const [showAlertPendingInQueue, setShowAlertPendingInQueue] = useState(false);
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

  return (
    <>
      <FullWidthPageHeader>
        <Stack direction="row" spacing={3} alignItems="center">
          <Avatar
            size="md"
            src={workspace.template_icon}
            variant={workspace.template_icon ? "square" : undefined}
            fitImage={Boolean(workspace.template_icon)}
          >
            {workspace.name}
          </Avatar>
          <div>
            <PageHeaderTitle>{workspace.name}</PageHeaderTitle>
            <PageHeaderSubtitle>{workspace.owner_name}</PageHeaderSubtitle>
          </div>
        </Stack>

        <WorkspaceStats
          workspace={workspace}
          handleUpdate={handleUpdate}
          canUpdateWorkspace={canUpdateWorkspace}
          maxDeadlineDecrease={scheduleProps.maxDeadlineDecrease}
          maxDeadlineIncrease={scheduleProps.maxDeadlineIncrease}
          onDeadlineMinus={scheduleProps.onDeadlineMinus}
          onDeadlinePlus={scheduleProps.onDeadlinePlus}
        />

        {canUpdateWorkspace && (
          <PageHeaderActions>
            <WorkspaceActions
              workspace={workspace}
              handleStart={handleStart}
              handleStop={handleStop}
              handleRestart={handleRestart}
              handleDelete={handleDelete}
              handleUpdate={handleUpdate}
              handleCancel={handleCancel}
              handleSettings={handleSettings}
              handleChangeVersion={handleChangeVersion}
              handleDormantActivate={handleDormantActivate}
              canChangeVersions={canChangeVersions}
              isUpdating={isUpdating}
              isRestarting={isRestarting}
            />
          </PageHeaderActions>
        )}
      </FullWidthPageHeader>

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
          {buildError}
          {cancellationError}
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
            workspaces={[workspace]}
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
                canRetryDebugMode && (
                  <Button
                    key={0}
                    onClick={handleBuildRetry}
                    variant="text"
                    size="small"
                  >
                    Try in debug mode
                  </Button>
                )
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
                  serverVersion={serverVersion}
                  onUpdateAgent={handleUpdate} // On updating the workspace the agent version is also updated
                />
              )}
            />
          )}

          {workspaceErrors[WorkspaceErrors.GET_BUILDS_ERROR] ? (
            <ErrorAlert
              error={workspaceErrors[WorkspaceErrors.GET_BUILDS_ERROR]}
            />
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
