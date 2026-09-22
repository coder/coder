import { PencilIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { updateChatProject } from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ChatProjectIcon } from "./ChatProjectIcon";
import { ChatProjectDialog } from "./ChatsSidebar/dialogs/ChatProjectDialog";

type ProjectComposerHeaderProps = {
	readonly project: ChatProject;
};

/**
 * Replaces the generic new-chat headline with the project identity so the
 * composer reads as "a new chat in this project".
 */
export const ProjectComposerHeader: FC<ProjectComposerHeaderProps> = ({
	project,
}) => (
	<div className="mb-4 text-center">
		<h1 className="m-0 flex items-center justify-center gap-2 text-2xl font-semibold text-content-primary">
			{project.icon && <ChatProjectIcon project={project} className="size-7" />}
			{project.name}
		</h1>
		{project.description && (
			<p className="mx-auto mb-0 mt-2 max-w-xl text-sm text-content-secondary">
				{project.description}
			</p>
		)}
	</div>
);

type ProjectComposerFooterProps = {
	readonly project: ChatProject;
};

/** Edit control under the composer. */
export const ProjectComposerFooter: FC<ProjectComposerFooterProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const [isEditing, setIsEditing] = useState(false);
	const updateProjectMutation = useMutation(updateChatProject(queryClient));

	return (
		<div className="flex justify-center pt-2">
			<Button
				variant="subtle"
				size="sm"
				className="text-content-secondary"
				onClick={() => setIsEditing(true)}
			>
				<PencilIcon />
				Edit project
			</Button>
			<ChatProjectDialog
				organizationId={project.organization_id}
				project={project}
				open={isEditing}
				onOpenChange={setIsEditing}
				onSubmit={async (request) => {
					await updateProjectMutation.mutateAsync({
						projectId: project.id,
						request,
					});
				}}
			/>
		</div>
	);
};
