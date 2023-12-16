import { Theme } from "@mui/material/styles";
import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";
import minMax from "dayjs/plugin/minMax";
import utc from "dayjs/plugin/utc";
import semver from "semver";
import * as TypesGen from "api/typesGenerated";
import CircularProgress from "@mui/material/CircularProgress";
import ErrorIcon from "@mui/icons-material/ErrorOutline";
import StopIcon from "@mui/icons-material/StopOutlined";
import PlayIcon from "@mui/icons-material/PlayArrowOutlined";
import QueuedIcon from "@mui/icons-material/HourglassEmpty";

dayjs.extend(duration);
dayjs.extend(utc);
dayjs.extend(minMax);

const DisplayWorkspaceBuildStatusLanguage = {
  succeeded: "Succeeded",
  pending: "Pending",
  running: "Running",
  canceling: "Canceling",
  canceled: "Canceled",
  failed: "Failed",
};

const DisplayAgentVersionLanguage = {
  unknown: "Unknown",
};

export const getDisplayWorkspaceBuildStatus = (
  theme: Theme,
  build: TypesGen.WorkspaceBuild,
) => {
  switch (build.job.status) {
    case "succeeded":
      return {
        type: "success",
        color: theme.palette.success.light,
        status: DisplayWorkspaceBuildStatusLanguage.succeeded,
      } as const;
    case "pending":
      return {
        type: "secondary",
        color: theme.palette.text.secondary,
        status: DisplayWorkspaceBuildStatusLanguage.pending,
      } as const;
    case "running":
      return {
        type: "info",
        color: theme.palette.primary.main,
        status: DisplayWorkspaceBuildStatusLanguage.running,
      } as const;
    // Just handle unknown as failed
    case "unknown":
    case "failed":
      return {
        type: "error",
        color: theme.palette.text.secondary,
        status: DisplayWorkspaceBuildStatusLanguage.failed,
      } as const;
    case "canceling":
      return {
        type: "warning",
        color: theme.palette.warning.light,
        status: DisplayWorkspaceBuildStatusLanguage.canceling,
      } as const;
    case "canceled":
      return {
        type: "secondary",
        color: theme.palette.text.secondary,
        status: DisplayWorkspaceBuildStatusLanguage.canceled,
      } as const;
  }
};

export const getDisplayWorkspaceBuildInitiatedBy = (
  build: TypesGen.WorkspaceBuild,
): string => {
  switch (build.reason) {
    case "initiator":
      return build.initiator_name;
    case "autostart":
    case "autostop":
      return "Coder";
  }
};

const getWorkspaceBuildDurationInSeconds = (
  build: TypesGen.WorkspaceBuild,
): number | undefined => {
  const isCompleted = build.job.started_at && build.job.completed_at;

  if (!isCompleted) {
    return;
  }

  const startedAt = dayjs(build.job.started_at);
  const completedAt = dayjs(build.job.completed_at);
  return completedAt.diff(startedAt, "seconds");
};

export const displayWorkspaceBuildDuration = (
  build: TypesGen.WorkspaceBuild,
  inProgressLabel = "In progress",
): string => {
  const duration = getWorkspaceBuildDurationInSeconds(build);
  return duration ? `${duration} seconds` : inProgressLabel;
};

export const enum agentVersionStatus {
  Updated = 1,
  Outdated = 2,
  Deprecated = 3,
}

export const getDisplayVersionStatus = (
  agentVersion: string,
  serverVersion: string,
  agentAPIVersion: string,
  serverAPIVersion: string,
): { displayVersion: string; status: agentVersionStatus } => {
  // APIVersions only have major.minor so coerce them to major.minor.0, so we can use semver.major()
  const a = semver.coerce(agentAPIVersion);
  const s = semver.coerce(serverAPIVersion);
  let status = agentVersionStatus.Updated;
  if (
    semver.valid(agentVersion) &&
    semver.valid(serverVersion) &&
    semver.lt(agentVersion, serverVersion)
  ) {
    status = agentVersionStatus.Outdated;
  }
  // deprecated overrides and implies Outdated
  if (a !== null && s !== null && semver.major(a) < semver.major(s)) {
    status = agentVersionStatus.Deprecated;
  }
  const displayVersion = agentVersion || DisplayAgentVersionLanguage.unknown;
  return {
    displayVersion: displayVersion,
    status: status,
  };
};

export const isWorkspaceOn = (workspace: TypesGen.Workspace): boolean => {
  const transition = workspace.latest_build.transition;
  const status = workspace.latest_build.job.status;
  return transition === "start" && status === "succeeded";
};

export const defaultWorkspaceExtension = (
  __startDate?: dayjs.Dayjs,
): TypesGen.PutExtendWorkspaceRequest => {
  const now = __startDate ? dayjs(__startDate) : dayjs();
  const fourHoursFromNow = now.add(4, "hours").utc();

  return {
    deadline: fourHoursFromNow.format(),
  };
};

export const getDisplayWorkspaceTemplateName = (
  workspace: TypesGen.Workspace,
): string => {
  return workspace.template_display_name.length > 0
    ? workspace.template_display_name
    : workspace.template_name;
};

export const getDisplayWorkspaceStatus = (
  workspaceStatus: TypesGen.WorkspaceStatus,
  provisionerJob?: TypesGen.ProvisionerJob,
) => {
  switch (workspaceStatus) {
    case undefined:
      return {
        text: "Loading",
        icon: <LoadingIcon />,
      } as const;
    case "running":
      return {
        type: "success",
        text: "Running",
        icon: <PlayIcon />,
      } as const;
    case "starting":
      return {
        type: "active",
        text: "Starting",
        icon: <LoadingIcon />,
      } as const;
    case "stopping":
      return {
        type: "notice",
        text: "Stopping",
        icon: <LoadingIcon />,
      } as const;
    case "stopped":
      return {
        type: "notice",
        text: "Stopped",
        icon: <StopIcon />,
      } as const;
    case "deleting":
      return {
        type: "danger",
        text: "Deleting",
        icon: <LoadingIcon />,
      } as const;
    case "deleted":
      return {
        type: "danger",
        text: "Deleted",
        icon: <ErrorIcon />,
      } as const;
    case "canceling":
      return {
        type: "notice",
        text: "Canceling",
        icon: <LoadingIcon />,
      } as const;
    case "canceled":
      return {
        type: "notice",
        text: "Canceled",
        icon: <ErrorIcon />,
      } as const;
    case "failed":
      return {
        type: "error",
        text: "Failed",
        icon: <ErrorIcon />,
      } as const;
    case "pending":
      return {
        type: "info",
        text: getPendingWorkspaceStatusText(provisionerJob),
        icon: <QueuedIcon />,
      } as const;
  }
};

const getPendingWorkspaceStatusText = (
  provisionerJob?: TypesGen.ProvisionerJob,
): string => {
  if (!provisionerJob || provisionerJob.queue_size === 0) {
    return "Pending";
  }
  return "Position in queue: " + provisionerJob.queue_position;
};

const LoadingIcon = () => {
  return <CircularProgress size={10} style={{ color: "#FFF" }} />;
};

export const hasJobError = (workspace: TypesGen.Workspace) => {
  return workspace.latest_build.job.error !== undefined;
};

export const paramsUsedToCreateWorkspace = (
  param: TypesGen.TemplateVersionParameter,
) => !param.ephemeral;

export const getMatchingAgentOrFirst = (
  workspace: TypesGen.Workspace,
  agentName: string | undefined,
): TypesGen.WorkspaceAgent | undefined => {
  return workspace.latest_build.resources
    .map((resource) => {
      if (!resource.agents || resource.agents.length === 0) {
        return;
      }
      if (!agentName) {
        return resource.agents[0];
      }
      return resource.agents.find((agent) => agent.name === agentName);
    })
    .filter((a) => a)[0];
};

export const workspaceUpdatePolicy = (
  workspace: TypesGen.Workspace,
  canChangeVersions: boolean,
): TypesGen.AutomaticUpdates => {
  // If a template requires the active version and you cannot change versions
  // (restricted to template admins), then your policy must be "Always".
  if (workspace.template_require_active_version && !canChangeVersions) {
    return "always";
  }
  return workspace.automatic_updates;
};
