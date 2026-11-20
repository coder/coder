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
			 * Command run when the PTY session is first created. Reattaching to a
			 * live session does not run it again, but connecting after it has
			 * exited does.
			 */
			initialCommand?: string;
			/** Whether the command's session ended, so the tab must not reconnect. */
			sessionEnded?: boolean;
			/** ID of the command app that opened this terminal, used to dedupe tabs. */
			sourceAppId?: string;
	  }
	| {
			id: string;
			kind: "workspace_app";
			label: string;
			appId: string;
			agentId: string;
	  }
	| {
			id: string;
			kind: "port";
			label: string;
			agentId: string;
			port: number;
			protocol: WorkspaceAgentPortShareProtocol;
	  };

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
			(record.sessionEnded === undefined ||
				typeof record.sessionEnded === "boolean") &&
			(record.sourceAppId === undefined ||
				typeof record.sourceAppId === "string")
		);
	}

	if (record.kind === "workspace_app") {
		return (
			typeof record.label === "string" &&
			typeof record.appId === "string" &&
			typeof record.agentId === "string"
		);
	}

	if (record.kind === "port") {
		return (
			typeof record.label === "string" &&
			typeof record.agentId === "string" &&
			typeof record.port === "number" &&
			Number.isInteger(record.port) &&
			record.port >= 1 &&
			record.port <= 65535 &&
			(record.protocol === "http" || record.protocol === "https")
		);
	}

	return false;
}

/**
 * Records on a terminal tab whether its command's session ended. Every other
 * tab is returned as it is.
 */
export function setTerminalTabSessionEnded(
	tabs: readonly UserRightPanelTab[],
	tabId: string,
	sessionEnded: boolean,
): UserRightPanelTab[] {
	return tabs.map((tab) =>
		tab.id === tabId && tab.kind === "terminal"
			? { ...tab, sessionEnded }
			: tab,
	);
}

export function validateUserRightPanelTabs(
	tabs: readonly UserRightPanelTab[],
	{
		workspace,
		workspaceAgent,
		wildcardHostname,
	}: ValidateUserRightPanelTabsOptions,
): UserRightPanelTab[] {
	return tabs.filter((tab) => {
		if (tab.kind === "terminal") {
			return workspace !== undefined && workspaceAgent !== undefined;
		}

		if (!workspace) {
			return false;
		}

		if (tab.kind === "workspace_app") {
			const app = findWorkspaceAppWithAgent(workspace, tab.agentId, tab.appId);
			// agent-browser already has the built-in Browser tab.
			return (
				app !== undefined &&
				app.slug !== AGENT_BROWSER_APP_SLUG &&
				isWorkspaceAppEmbeddable(app)
			);
		}

		// Mirror the add-menu gate so a persisted port tab disappears when
		// the agent stops exposing the port forwarding helper.
		const agent = findWorkspaceAgent(workspace, tab.agentId);
		return (
			agent !== undefined && canShowPortForwarding(agent, wildcardHostname)
		);
	});
}
