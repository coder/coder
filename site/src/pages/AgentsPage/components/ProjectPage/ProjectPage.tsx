import type { ChatProject } from "#/api/typesGenerated";
import { ProjectChatsList } from "./ProjectChatsList";
import { ProjectDetailsPanel } from "./ProjectDetailsPanel";
import { ProjectHeaderSection } from "./ProjectHeaderSection";

type ProjectPageProps = {
	readonly project: ChatProject;
	readonly children: React.ReactNode;
};

export const ProjectPage: React.FC<ProjectPageProps> = ({
	project,
	children,
}) => (
	// flex-1 and min-h-0 bound the page to the layout's height at every
	// breakpoint, so a long chat list scrolls here instead of being clipped.
	<div className="order-last flex min-h-0 flex-1 flex-col overflow-auto px-4 pt-6 sm:order-0 sm:px-[85.5px]">
		<div className="mx-auto flex w-full max-w-6xl flex-col gap-6 pb-8 @container">
			<ProjectHeaderSection key={project.id} project={project} />
			<div className="flex flex-col gap-14">
				{children}
				{/* Columns follow the page width rather than the viewport, which
				    the agents sidebar narrows. */}
				<div className="grid grid-cols-1 gap-8 @4xl:grid-cols-[minmax(0,554fr)_minmax(0,310fr)]">
					<ProjectChatsList project={project} />
					<ProjectDetailsPanel project={project} />
				</div>
			</div>
		</div>
	</div>
);
