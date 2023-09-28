import Button from "@mui/material/Button";
import { makeStyles } from "@mui/styles";
import { Avatar } from "components/Avatar/Avatar";
import { AgentRow } from "components/Resources/AgentRow";
import {
  ActiveTransition,
  WorkspaceBuildProgress,
} from "./WorkspaceBuildProgress";
import { FC, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import * as TypesGen from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { BuildsTable } from "./BuildsTable";
import { Margins } from "components/Margins/Margins";
import { Resources } from "components/Resources/Resources";
import { Stack } from "components/Stack/Stack";
import { WorkspaceActions } from "pages/WorkspacePage/WorkspaceActions/WorkspaceActions";
import { WorkspaceDeletedBanner } from "./WorkspaceDeletedBanner";
import { WorkspaceStats } from "./WorkspaceStats";
import {
  FullWidthPageHeader,
  PageHeaderActions,
  PageHeaderTitle,
  PageHeaderSubtitle,
} from "components/PageHeader/FullWidthPageHeader";
import { TemplateVersionWarnings } from "components/TemplateVersionWarnings/TemplateVersionWarnings";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { DormantWorkspaceBanner } from "components/WorkspaceDeletion";
import { useLocalStorage } from "hooks";
import AlertTitle from "@mui/material/AlertTitle";
import dayjs from "dayjs";

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
  builds?: TypesGen.WorkspaceBuild[];
  templateWarnings?: TypesGen.TemplateVersionWarning[];
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
  quotaBudget,
  handleBuildRetry,
  templateWarnings,
  buildLogs,
}) => {
  const styles = useStyles();
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
          quotaBudget={quotaBudget}
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

      <Margins className={styles.content}>
        <Stack
          direction="column"
          className={styles.firstColumnSpacer}
          spacing={4}
        >
          {workspace.outdated && (
            <Alert severity="info">
              <AlertTitle>An update is available for your workspace</AlertTitle>
              {updateMessage && <AlertDetail>{updateMessage}</AlertDetail>}
            </Alert>
          )}
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

          <TemplateVersionWarnings warnings={templateWarnings} />

          {showAlertPendingInQueue && (
            <Alert severity="info">
              <AlertTitle>Workspace build is pending</AlertTitle>
              <AlertDetail>
                <div className={styles.alertPendingInQueue}>
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
            <BuildsTable builds={builds} />
          )}
        </Stack>
      </Margins>
    </>
  );
};

const spacerWidth = 300;

export const useStyles = makeStyles((theme) => {
  return {
    content: {
      marginTop: theme.spacing(4),
    },

    statusBadge: {
      marginLeft: theme.spacing(2),
    },

    actions: {
      [theme.breakpoints.down("md")]: {
        flexDirection: "column",
      },
    },

    firstColumnSpacer: {
      flex: 2,
    },

    secondColumnSpacer: {
      flex: `0 0 ${spacerWidth}px`,
    },

    layout: {
      alignItems: "flex-start",
    },

    main: {
      width: "100%",
    },

    timelineContents: {
      margin: 0,
    },
    logs: {
      border: `1px solid ${theme.palette.divider}`,
    },

    errorDetails: {
      color: theme.palette.text.secondary,
      fontSize: 12,
    },

    fullWidth: {
      width: "100%",
    },

    alertPendingInQueue: {
      marginBottom: 12,
    },
  };
});
