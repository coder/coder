import type { ChatProject } from "#/api/typesGenerated";
import { ProjectChatsList } from "./ProjectChatsList";
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
	<div className="order-last flex min-h-0 flex-1 flex-col overflow-auto px-4 pt-8 sm:order-0 sm:pt-16">
		<div className="mx-auto flex w-full max-w-5xl flex-col gap-6 pb-8">
			<ProjectHeaderSection key={project.id} project={project} />
			{children}
			<ProjectChatsList project={project} />
		</div>
	</div>
);
