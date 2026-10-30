import type { Workspace } from "#/api/typesGenerated";
import { WorkspaceAppFrame } from "#/modules/apps/WorkspaceAppFrame";
import type { WorkspaceAppWithAgent } from "#/modules/apps/workspaceApps";
import { useTabPanelState } from "../ChatsSidebar/tabs/SidebarTabView";

type AppPanelProps = {
	workspace: Workspace;
	app: WorkspaceAppWithAgent;
};

/**
 * An embedded workspace app. The frame stays mounted while its tab is
 * inactive, so the app keeps its state.
 */
export const AppPanel: React.FC<AppPanelProps> = ({ workspace, app }) => {
	const { isActive } = useTabPanelState();

	return (
		<WorkspaceAppFrame workspace={workspace} app={app} active={isActive} />
	);
};
