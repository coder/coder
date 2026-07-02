import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentPortShareProtocol,
} from "#/api/typesGenerated";
import { isWorkspaceAppEmbeddable } from "#/modules/apps/apps";
import { findWorkspaceAppWithAgent } from "#/modules/apps/workspaceApps";
import { canShowPortForwarding } from "#/modules/resources/usePortsData";
import { findWorkspaceAgent } from "#/utils/workspace";

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
			return app !== undefined && isWorkspaceAppEmbeddable(app);
		}

		// Mirror the add-menu gate so a persisted port tab disappears when
		// the agent stops exposing the port forwarding helper.
		const agent = findWorkspaceAgent(workspace, tab.agentId);
		return (
			agent !== undefined && canShowPortForwarding(agent, wildcardHostname)
		);
	});
}
