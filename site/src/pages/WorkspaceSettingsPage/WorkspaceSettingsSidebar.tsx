import type { FC } from "react";
import { useWorkspaceSettings } from "./useWorkspaceSettings";
import { WorkspaceSettingsSidebarView } from "./WorkspaceSettingsSidebarView";

/** Wires the workspace settings sidebar to the layout's workspace context. */
export const WorkspaceSettingsSidebar: FC = () => {
	const { workspace, permissions } = useWorkspaceSettings();

	return (
		<WorkspaceSettingsSidebarView
			workspace={workspace}
			canShareWorkspace={permissions?.shareWorkspace ?? false}
		/>
	);
};
