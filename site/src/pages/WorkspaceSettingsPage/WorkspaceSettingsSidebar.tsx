import type { FC } from "react";
import { useWorkspaceSettings } from "./useWorkspaceSettings";
import { WorkspaceSettingsSidebarView } from "./WorkspaceSettingsSidebarView";

/** Connects WorkspaceSettingsSidebarView to the workspace settings context. */
export const WorkspaceSettingsSidebar: FC = () => {
	const { workspace, permissions } = useWorkspaceSettings();

	return (
		<WorkspaceSettingsSidebarView
			workspace={workspace}
			canShareWorkspace={permissions?.shareWorkspace ?? false}
		/>
	);
};
