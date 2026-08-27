import type { ComponentProps, FC } from "react";
import { WorkspaceStatusIndicator } from "#/modules/workspaces/WorkspaceStatusIndicator/WorkspaceStatusIndicator";
import { WorkspaceActions } from "./WorkspaceActions/WorkspaceActions";

type WorkspaceHeaderProps = ComponentProps<typeof WorkspaceActions>;

export const WorkspaceHeader: FC<WorkspaceHeaderProps> = (props) => {
	const { workspace } = props;

	return (
		<div className="flex flex-col md:flex-row flex-wrap items-start justify-between gap-4">
			<div className="flex items-center flex-wrap gap-6">
				<h1 className="text-3xl font-semibold">{workspace.name}</h1>
				<WorkspaceStatusIndicator workspace={workspace} />
			</div>
			<WorkspaceActions {...props} />
		</div>
	);
};
