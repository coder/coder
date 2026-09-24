import type { FC } from "react";
import type { ChatProject } from "#/api/typesGenerated";
import { ChatProjectIcon } from "./ChatProjectIcon";

type ProjectComposerHeaderProps = {
	readonly project: ChatProject;
};

/** Project name and description shown above the new-chat composer. */
export const ProjectComposerHeader: FC<ProjectComposerHeaderProps> = ({
	project,
}) => (
	<div className="mb-4 min-w-0 text-center">
		<h1 className="m-0 flex items-center justify-center gap-2 break-words text-2xl font-semibold text-content-primary [overflow-wrap:anywhere]">
			{project.icon && <ChatProjectIcon project={project} className="size-7" />}
			<span className="min-w-0">{project.name}</span>
		</h1>
		{project.description && (
			<p className="mx-auto mb-0 mt-2 max-w-xl break-words text-sm text-content-secondary [overflow-wrap:anywhere]">
				{project.description}
			</p>
		)}
	</div>
);
