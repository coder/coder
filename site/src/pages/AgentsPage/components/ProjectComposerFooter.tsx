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

/** "Edit project" button and the dialog that saves the edit. */
export const ProjectComposerFooter: React.FC<ProjectComposerFooterProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const [isDialogOpen, setIsDialogOpen] = useState(false);
	const updateProjectMutation = useMutation(updateChatProject(queryClient));

	return (
		<div className="flex justify-center pt-2">
			<Button
				variant="subtle"
				size="sm"
				onClick={() => {
					updateProjectMutation.reset();
					setIsDialogOpen(true);
				}}
			>
				<PencilIcon />
				Edit project
			</Button>
			<ChatProjectDialog
				project={project}
				open={isDialogOpen}
				onOpenChange={setIsDialogOpen}
				isSubmitting={updateProjectMutation.isPending}
				error={updateProjectMutation.error}
				onSubmit={(request) => {
					updateProjectMutation.mutate(
						{ project, request },
						{ onSuccess: () => setIsDialogOpen(false) },
					);
				}}
			/>
		</div>
	);
};
