import type { Theme } from "@emotion/react";
import { CircleAlertIcon } from "lucide-react";
import QueuedIcon from "@mui/icons-material/HourglassEmpty";
import PlayIcon from "@mui/icons-material/PlayArrowOutlined";
import StopIcon from "@mui/icons-material/StopOutlined";
import type * as TypesGen from "api/typesGenerated";
import { PillSpinner } from "components/Pill/Pill";
import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";
import minMax from "dayjs/plugin/minMax";
import utc from "dayjs/plugin/utc";
import semver from "semver";
import { getPendingStatusLabel } from "./provisionerJob";

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
				color: theme.roles.success.text,
				status: DisplayWorkspaceBuildStatusLanguage.succeeded,
			} as const;
		case "pending":
			return {
				type: "inactive",
				color: theme.roles.active.text,
				status: DisplayWorkspaceBuildStatusLanguage.pending,
			} as const;
		case "running":
			return {
				type: "active",
				color: theme.roles.active.text,
				status: DisplayWorkspaceBuildStatusLanguage.running,
			} as const;
		// Just handle unknown as failed
		case "unknown":
		case "failed":
			return {
				type: "error",
				color: theme.roles.error.text,
				status: DisplayWorkspaceBuildStatusLanguage.failed,
			} as const;
		case "canceling":
			return {
				type: "warning",
				color: theme.roles.warning.text,
				status: DisplayWorkspaceBuildStatusLanguage.canceling,
			} as const;
		case "canceled":
			return {
				type: "inactive",
				color: theme.roles.warning.text,
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

export enum agentVersionStatus {
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

export type DisplayWorkspaceStatusType =
	| "success"
	| "active"
	| "inactive"
	| "error"
	| "warning"
	| "danger";

type DisplayWorkspaceStatus = {
	text: string;
	type: DisplayWorkspaceStatusType;
	icon: React.ReactNode;
};

export const getDisplayWorkspaceStatus = (
	workspaceStatus: TypesGen.WorkspaceStatus,
	provisionerJob?: TypesGen.ProvisionerJob,
): DisplayWorkspaceStatus => {
	switch (workspaceStatus) {
		case undefined:
			return {
				text: "Loading",
				type: "active",
				icon: <PillSpinner />,
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
				icon: <PillSpinner />,
			} as const;
		case "stopping":
			return {
				type: "inactive",
				text: "Stopping",
				icon: <PillSpinner />,
			} as const;
		case "stopped":
			return {
				type: "inactive",
				text: "Stopped",
				icon: <StopIcon />,
			} as const;
		case "deleting":
			return {
				type: "danger",
				text: "Deleting",
				icon: <PillSpinner />,
			} as const;
		case "deleted":
			return {
				type: "danger",
				text: "Deleted",
				icon: <CircleAlertIcon aria-hidden="true" className="size-icon-sm" />,
			} as const;
		case "canceling":
			return {
				type: "inactive",
				text: "Canceling",
				icon: <PillSpinner />,
			} as const;
		case "canceled":
			return {
				type: "inactive",
				text: "Canceled",
				icon: <CircleAlertIcon aria-hidden="true" className="size-icon-sm" />,
			} as const;
		case "failed":
			return {
				type: "error",
				text: "Failed",
				icon: <CircleAlertIcon aria-hidden="true" className="size-icon-sm" />,
			} as const;
		case "pending":
			return {
				type: "active",
				text: getPendingStatusLabel(provisionerJob),
				icon: <QueuedIcon />,
			} as const;
	}
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

export const mustUpdateWorkspace = (
	workspace: TypesGen.Workspace,
	canChangeVersions: boolean,
): boolean => {
	return (
		workspaceUpdatePolicy(workspace, canChangeVersions) === "always" &&
		workspace.outdated
	);
};

const workspaceUpdatePolicy = (
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

// These resources (i.e. docker_image, kubernetes_deployment) map to Terraform
// resource types. These are the most used ones and are based on user usage.
// We may want to update from time-to-time.
const BUILT_IN_ICON_PATHS: Record<string, `/icon/${string}`> = {
	docker_volume: "/icon/database.svg",
	docker_container: "/icon/memory.svg",
	docker_image: "/icon/container.svg",
	kubernetes_persistent_volume_claim: "/icon/database.svg",
	kubernetes_pod: "/icon/memory.svg",
	google_compute_disk: "/icon/database.svg",
	google_compute_instance: "/icon/memory.svg",
	aws_instance: "/icon/memory.svg",
	kubernetes_deployment: "/icon/memory.svg",
};
const FALLBACK_ICON = "/icon/widgets.svg";

export const getResourceIconPath = (resourceType: string): string => {
	return BUILT_IN_ICON_PATHS[resourceType] ?? FALLBACK_ICON;
};

export const lastUsedMessage = (lastUsedAt: string | Date): string => {
	const t = dayjs(lastUsedAt);
	const now = dayjs();
	let message = t.fromNow();

	if (t.isAfter(now.subtract(1, "hour"))) {
		message = "Now";
	} else if (t.isAfter(now.subtract(3, "day"))) {
		message = t.fromNow();
	} else if (t.isAfter(now.subtract(1, "month"))) {
		message = t.fromNow();
	} else if (t.isAfter(now.subtract(100, "year"))) {
		message = t.fromNow();
	} else {
		message = "Never";
	}

	return message;
};
