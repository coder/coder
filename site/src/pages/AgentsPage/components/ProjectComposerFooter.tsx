import { PencilIcon } from "lucide-react";
import { useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { updateChatProject } from "#/api/queries/chatProjects";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ChatProjectDialog } from "./ChatsSidebar/dialogs/ChatProjectDialog";

type ProjectComposerFooterProps = {
	readonly project: TypesGen.ChatProject;
};

/** "Edit project" button shown below the new-chat composer on a project page. */
export const ProjectComposerFooter: React.FC<ProjectComposerFooterProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const [isEditing, setIsEditing] = useState(false);
	const updateProjectMutation = useMutation(updateChatProject(queryClient));
	const closeDialog = () => setIsEditing(false);

	return (
		<div className="flex justify-center pt-2">
			<Button
				variant="subtle"
				size="sm"
				onClick={() => {
					updateProjectMutation.reset();
					setIsEditing(true);
				}}
			>
				<PencilIcon />
				Edit project
			</Button>
			<ChatProjectDialog
				project={project}
				open={isEditing}
				onOpenChange={(open) => {
					if (!open) closeDialog();
				}}
				isSubmitting={updateProjectMutation.isPending}
				error={updateProjectMutation.error}
				onSubmit={(request) => {
					updateProjectMutation.mutate(
						{ project, request },
						{ onSuccess: closeDialog },
					);
				}}
			/>
		</div>
	);
};
