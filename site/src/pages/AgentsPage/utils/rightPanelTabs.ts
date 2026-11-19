import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentPortShareProtocol,
} from "#/api/typesGenerated";
import {
	AGENT_BROWSER_APP_SLUG,
	isWorkspaceAppEmbeddable,
} from "#/modules/apps/apps";
import { findWorkspaceAppWithAgent } from "#/modules/apps/workspaceApps";
import { canShowPortForwarding } from "#/modules/resources/usePortsData";
import { isFilePreviewID, isValidPort } from "#/utils/portForward";
import { findWorkspaceAgent } from "#/utils/workspace";

/**
 * Built-in panels that the user shows or hides from the add-tab dropdown.
 * Each one can appear at most once, so the tab ID is the panel ID.
 */
export const singletonRightPanelTabIds = [
	"browser",
	"desktop",
	"debug",
] as const;

export type SingletonRightPanelTabId =
	(typeof singletonRightPanelTabIds)[number];

export function isSingletonRightPanelTabId(
	value: unknown,
): value is SingletonRightPanelTabId {
	return singletonRightPanelTabIds.some((id) => id === value);
}

export type PortSelection = {
	label: string;
	port: number;
	protocol: WorkspaceAgentPortShareProtocol;
};

export type UserRightPanelTab =
	| {
			id: string;
			kind: "terminal";
			label?: string;
			reconnectionToken: string;
			/**
			 * Command run when the PTY session is first created. The backend only
			 * runs it for a fresh reconnect token, so reattaching does not re-run it.
			 */
			initialCommand?: string;
			/** ID of the command app that opened this terminal, used to dedupe tabs. */
			sourceAppId?: string;
	  }
	| {
			id: string;
			kind: "workspace_app";
			label: string;
			appId: string;
			agentId: string;
			urlPath?: string;
	  }
	| {
			id: string;
			kind: "port";
			label: string;
			agentId: string;
			port: number;
			protocol: WorkspaceAgentPortShareProtocol;
			urlPath?: string;
	  }
	| {
			id: string;
			kind: "file_preview";
			label: string;
			agentId: string;
			previewId: string;
			urlPath?: string;
	  };

type WithoutID<T> = T extends unknown ? Omit<T, "id"> : never;

/** An app, port, or file preview tab before it has an ID. */
export type NewFrameTab = WithoutID<
	Extract<
		UserRightPanelTab,
		{ kind: "workspace_app" | "port" | "file_preview" }
	>
>;

type ValidateUserRightPanelTabsOptions = {
	workspace: Workspace | undefined;
	workspaceAgent: WorkspaceAgent | undefined;
	wildcardHostname: string;
};

export function isUserRightPanelTab(
	value: unknown,
): value is UserRightPanelTab {
	if (typeof value !== "object" || value === null) {
		return false;
	}
	const record = value as Record<string, unknown>;
	if (typeof record.id !== "string") {
		return false;
	}

	if (record.kind === "terminal") {
		return (
			typeof record.reconnectionToken === "string" &&
			(record.label === undefined || typeof record.label === "string") &&
			(record.initialCommand === undefined ||
				typeof record.initialCommand === "string") &&
			(record.sourceAppId === undefined ||
				typeof record.sourceAppId === "string")
		);
	}

	const hasValidURLPath =
		record.urlPath === undefined || typeof record.urlPath === "string";

	if (record.kind === "workspace_app") {
		return (
			typeof record.label === "string" &&
			typeof record.appId === "string" &&
			typeof record.agentId === "string" &&
			hasValidURLPath
		);
	}

	if (record.kind === "port") {
		return (
			typeof record.label === "string" &&
			typeof record.agentId === "string" &&
			isValidPort(record.port) &&
			(record.protocol === "http" || record.protocol === "https") &&
			hasValidURLPath
		);
	}

	if (record.kind === "file_preview") {
		return (
			typeof record.label === "string" &&
			typeof record.agentId === "string" &&
			isFilePreviewID(record.previewId) &&
			hasValidURLPath
		);
	}

	return false;
}

export function validateUserRightPanelTabs(
	tabs: readonly UserRightPanelTab[],
	{
		workspace,
		workspaceAgent,
		wildcardHostname,
	}: ValidateUserRightPanelTabsOptions,
): UserRightPanelTab[] {
	return tabs.filter((tab) =>
		isUserRightPanelTabAvailable(tab, {
			workspace,
			workspaceAgent,
			wildcardHostname,
		}),
	);
}

/** Reports whether the workspace can still show the tab. */
export function isUserRightPanelTabAvailable(
	tab: WithoutID<UserRightPanelTab>,
	{
		workspace,
		workspaceAgent,
		wildcardHostname,
	}: ValidateUserRightPanelTabsOptions,
): boolean {
	if (tab.kind === "terminal") {
		return workspace !== undefined && workspaceAgent !== undefined;
	}

	if (!workspace) {
		return false;
	}

	switch (tab.kind) {
		case "workspace_app": {
			const app = findWorkspaceAppWithAgent(workspace, tab.agentId, tab.appId);
			// agent-browser already has the built-in Browser tab.
			return (
				app !== undefined &&
				app.slug !== AGENT_BROWSER_APP_SLUG &&
				isWorkspaceAppEmbeddable(app)
			);
		}
		case "port": {
			// Mirror the add-menu gate so a persisted port tab disappears when
			// the agent stops exposing the port forwarding helper.
			const agent = findWorkspaceAgent(workspace, tab.agentId);
			return (
				agent !== undefined && canShowPortForwarding(agent, wildcardHostname)
			);
		}
		case "file_preview":
			return (
				findWorkspaceAgent(workspace, tab.agentId) !== undefined &&
				wildcardHostname.trim() !== ""
			);
		default: {
			const _exhaustive: never = tab;
			return _exhaustive;
		}
	}
}

/**
 * Reports whether tab shows the same frame as target, so opening target
 * focuses tab instead of adding another.
 */
export function showsSameFrame(
	tab: UserRightPanelTab,
	target: NewFrameTab,
): boolean {
	switch (target.kind) {
		case "workspace_app":
			return (
				tab.kind === "workspace_app" &&
				tab.agentId === target.agentId &&
				tab.appId === target.appId &&
				tab.urlPath === target.urlPath
			);
		case "port":
			return (
				tab.kind === "port" &&
				tab.agentId === target.agentId &&
				tab.port === target.port &&
				tab.protocol === target.protocol &&
				tab.urlPath === target.urlPath
			);
		case "file_preview":
			return (
				tab.kind === "file_preview" &&
				tab.agentId === target.agentId &&
				tab.previewId === target.previewId &&
				tab.urlPath === target.urlPath
			);
		default: {
			const _exhaustive: never = target;
			return _exhaustive;
		}
	}
}
