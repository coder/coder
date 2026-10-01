import type { ChatProject } from "#/api/typesGenerated";
import { ChatProjectIcon } from "../ChatProjectIcon";

type ProjectPageHeaderProps = {
	readonly project: ChatProject;
	/** Controls placed after the project name, such as an actions menu. */
	readonly actions?: React.ReactNode;
	/** Shown below the description. */
	readonly metadata?: React.ReactNode;
};

/** The project's icon, name, description, and metadata. */
export const ProjectPageHeader: React.FC<ProjectPageHeaderProps> = ({
	project,
	actions,
	metadata,
}) => (
	<header className="flex min-w-0 flex-col gap-3">
		<div className="flex min-w-0 items-center gap-3">
			<div className="flex size-7 shrink-0 items-center justify-center rounded-md border border-solid border-border bg-surface-secondary">
				<ChatProjectIcon project={project} className="size-4" />
			</div>
			<h1 className="m-0 min-w-0 flex-1 text-3xl font-semibold text-content-primary [overflow-wrap:anywhere]">
				{project.name}
			</h1>
			{actions}
		</div>
		{project.description && (
			<p className="m-0 text-sm text-content-secondary [overflow-wrap:anywhere]">
				{project.description}
			</p>
		)}
		{metadata}
	</header>
);
