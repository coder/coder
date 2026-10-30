import type * as TypesGen from "#/api/typesGenerated";
import { findWorkspaceAppWithAgent } from "#/modules/apps/workspaceApps";
import { findWorkspaceAgent } from "#/utils/workspace";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import { TerminalPanel } from "../TerminalPanel";
import { AppPanel } from "./AppPanel";
import { PortPreviewPanel } from "./PortPreviewPanel";

type UserTabPanelProps = {
	tab: UserRightPanelTab;
	chatId: string;
	workspace: TypesGen.Workspace | undefined;
	workspaceAgent: TypesGen.WorkspaceAgent | undefined;
	wildcardHostname: string;
	isPending: boolean;
	onTerminalReady: (tabId: string) => void;
};

/** Renders a terminal, workspace app, or port tab. */
export const UserTabPanel: React.FC<UserTabPanelProps> = ({
	tab,
	chatId,
	workspace,
	workspaceAgent,
	wildcardHostname,
	isPending,
	onTerminalReady,
}) => {
	switch (tab.kind) {
		case "terminal":
			return workspace && workspaceAgent ? (
				<TerminalPanel
					chatId={chatId}
					reconnectionToken={tab.reconnectionToken}
					initialCommand={tab.initialCommand}
					isPending={isPending}
					onReady={() => onTerminalReady(tab.id)}
					workspace={workspace}
					workspaceAgent={workspaceAgent}
				/>
			) : null;
		// validateUserRightPanelTabs already dropped tabs whose app or agent is
		// gone, so these lookups only narrow types.
		case "workspace_app": {
			const app =
				workspace &&
				findWorkspaceAppWithAgent(workspace, tab.agentId, tab.appId);
			if (!workspace || !app) {
				return null;
			}
			return <AppPanel workspace={workspace} app={app} />;
		}
		case "port": {
			const agent = workspace && findWorkspaceAgent(workspace, tab.agentId);
			if (!workspace || !agent) {
				return null;
			}
			return (
				<PortPreviewPanel
					workspace={workspace}
					agent={agent}
					host={wildcardHostname}
					tab={tab}
				/>
			);
		}
		default: {
			const _exhaustive: never = tab;
			return _exhaustive;
		}
	}
};
