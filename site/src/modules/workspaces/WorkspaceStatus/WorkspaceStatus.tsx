import type { Workspace } from "api/typesGenerated";
import type { FC } from "react";
import { lastUsedMessage } from "utils/workspace";
import { WorkspaceDormantBadge } from "../WorkspaceDormantBadge/WorkspaceDormantBadge";
import { WorkspaceStatusIndicator } from "../WorkspaceStatusIndicator/WorkspaceStatusIndicator";

type WorkspaceStatusProps = {
	workspace: Workspace;
};

export const WorkspaceStatus: FC<WorkspaceStatusProps> = ({ workspace }) => {
	return (
		<div className="flex flex-col">
			<WorkspaceStatusIndicator workspace={workspace}>
				{workspace.dormant_at && (
					<WorkspaceDormantBadge workspace={workspace} />
				)}
			</WorkspaceStatusIndicator>
			<time
				dateTime={workspace.last_used_at}
				className="text-xs font-medium text-content-secondary ml-6 whitespace-nowrap"
			>
				{lastUsedMessage(workspace.last_used_at)}
			</time>
		</div>
	);
};
