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
 * Tabs in the right panel strip, in display order. The strip is fixed: a
 * tab appears when its content is available and is never closed by the
 * user. Terminals and app or port previews live one level down, inside the
 * Terminal and Workspace tabs.
 */
const rightPanelTabIds = [
	"summary",
	"git",
	"terminal",
	"browser",
	"desktop",
	"workspace",
	"debug",
] as const;

export type RightPanelTabId = (typeof rightPanelTabIds)[number];

function isRightPanelTabId(value: unknown): value is RightPanelTabId {
	return rightPanelTabIds.some((id) => id === value);
}

/**
 * Maps a persisted tab ID from the previous strip, where every terminal, app,
 * and port preview was its own top-level tab, onto the tab that now holds it.
 */
export function resolveRightPanelTabId(
	value: string | null,
): RightPanelTabId | null {
	if (value === null) {
		return null;
	}
	if (isRightPanelTabId(value)) {
		return value;
	}
	if (value.startsWith("terminal-")) {
		return "terminal";
	}
	if (value.startsWith("workspace_app-") || value.startsWith("port-")) {
		return "workspace";
	}
	return null;
}

/** Tabs that own a sub-layer of closeable chips. */
export type RightPanelGroupTabId = "terminal" | "workspace";

export type PortSelection = {
	label: string;
	port: number;
	protocol: WorkspaceAgentPortShareProtocol;
};

export type UserRightPanelTab =
	| TerminalRightPanelTab
	| WorkspacePreviewRightPanelTab;

type TerminalRightPanelTab = {
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
};

/** App and port previews shown as chips inside the Workspace tab. */
export type WorkspacePreviewRightPanelTab =
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

export function isTerminalRightPanelTab(
	tab: UserRightPanelTab,
): tab is TerminalRightPanelTab {
	return tab.kind === "terminal";
}

export function isWorkspacePreviewRightPanelTab(
	tab: UserRightPanelTab,
): tab is WorkspacePreviewRightPanelTab {
	return tab.kind === "workspace_app" || tab.kind === "port";
}

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
