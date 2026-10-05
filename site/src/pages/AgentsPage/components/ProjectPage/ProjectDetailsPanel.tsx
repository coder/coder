import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatProjectInstructions,
	deleteChatProjectInstructions,
	updateChatProjectInstructions,
} from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { ProjectDetailsPanelView } from "./ProjectDetailsPanelView";
import { ProjectInstructionsDialog } from "./ProjectInstructionsDialog";

type ProjectDetailsPanelProps = {
	readonly project: ChatProject;
};

/** Loads a project's details and edits its instructions. */
export const ProjectDetailsPanel: React.FC<ProjectDetailsPanelProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const instructionsQuery = useQuery(chatProjectInstructions(project));
	const updateMutation = useMutation(
		updateChatProjectInstructions(queryClient, project),
	);
	const deleteMutation = useMutation(
		deleteChatProjectInstructions(queryClient, project),
	);
	const [isEditorOpen, setIsEditorOpen] = useState(false);

	const openEditor = () => {
		updateMutation.reset();
		deleteMutation.reset();
		setIsEditorOpen(true);
	};

	return (
		<>
			<ProjectDetailsPanelView
				instructions={instructionsQuery.data}
				error={instructionsQuery.error}
				onRetry={() => void instructionsQuery.refetch()}
				onEditInstructions={openEditor}
			/>
			<ProjectInstructionsDialog
				open={isEditorOpen}
				onOpenChange={setIsEditorOpen}
				instructions={instructionsQuery.data?.instructions ?? ""}
				isSaving={updateMutation.isPending}
				isDeleting={deleteMutation.isPending}
				error={updateMutation.error ?? deleteMutation.error}
				onSave={(instructions) =>
					updateMutation.mutate(
						{ instructions },
						{ onSuccess: () => setIsEditorOpen(false) },
					)
				}
				onDelete={() =>
					deleteMutation.mutate(undefined, {
						onSuccess: () => setIsEditorOpen(false),
					})
				}
			/>
		</>
	);
};
