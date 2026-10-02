import type { ChatProject } from "#/api/typesGenerated";
import { ChatProjectIcon } from "../ChatProjectIcon";

type ProjectPageHeaderProps = {
	readonly project: ChatProject;
	readonly actions?: React.ReactNode;
	readonly metadata?: React.ReactNode;
};

export const ProjectPageHeader: React.FC<ProjectPageHeaderProps> = ({
	project,
	actions,
	metadata,
}) => (
	<header className="flex min-w-0 flex-col gap-1">
		<div className="flex min-w-0 items-center gap-3">
			<div className="flex size-7 shrink-0 items-center justify-center rounded-md border border-solid border-border bg-surface-secondary">
				<ChatProjectIcon project={project} className="size-4" />
			</div>
			<div className="flex min-w-0 items-center gap-4">
				<h1 className="m-0 min-w-0 truncate text-2xl font-semibold text-content-primary">
					{project.name}
				</h1>
				{actions}
			</div>
		</div>
		{metadata}
		{project.description && (
			<p className="m-0 text-sm text-content-secondary [overflow-wrap:anywhere]">
				{project.description}
			</p>
		)}
	</header>
);
