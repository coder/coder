import type { ChatProject } from "#/api/typesGenerated";
import { ProjectChatsList } from "./ProjectChatsList";
import { ProjectHeaderSection } from "./ProjectHeaderSection";

type ProjectPageProps = {
	readonly project: ChatProject;
	/** The new-chat composer. */
	readonly children: React.ReactNode;
};

/**
 * Lays out a chat project's page: its header, the new-chat composer, and the
 * project's chats.
 */
export const ProjectPage: React.FC<ProjectPageProps> = ({
	project,
	children,
}) => (
	// flex-1 and min-h-0 bound the page to the layout's height at every
	// breakpoint, so a long chat list scrolls here instead of being clipped.
	<div className="order-last flex min-h-0 flex-1 justify-center overflow-auto px-4 pb-4 pt-8 sm:order-0 sm:pt-16">
		<div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
			{/* Keyed so dialog state never carries over to another project. */}
			<ProjectHeaderSection key={project.id} project={project} />
			{children}
			<ProjectChatsList project={project} />
		</div>
	</div>
);
