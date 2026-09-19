import { PencilIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatProjectPermissions,
	updateChatProject,
} from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { chatProjectPermissionsFor } from "#/modules/permissions/chatProjects";
import { ProjectShareButton } from "./ChatSharingPopover";
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
		<h1 className="m-0 text-2xl font-semibold text-content-primary">
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

/**
 * Edit and share controls under the composer, shown only to users allowed
 * to manage the project.
 */
export const ProjectComposerFooter: FC<ProjectComposerFooterProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const [isEditing, setIsEditing] = useState(false);
	const permissionsQuery = useQuery(chatProjectPermissions([project]));
	const updateProjectMutation = useMutation(updateChatProject(queryClient));
	const { canUpdate, canShare } = chatProjectPermissionsFor(
		project,
		permissionsQuery.data,
	);

	if (!canUpdate && !canShare) {
		return null;
	}

	return (
		<div className="flex justify-center gap-2 pt-2">
			{canUpdate && (
				<Button
					variant="subtle"
					size="sm"
					className="text-content-secondary"
					onClick={() => setIsEditing(true)}
				>
					<PencilIcon />
					Edit project
				</Button>
			)}
			{canShare && (
				<ProjectShareButton
					projectId={project.id}
					organizationId={project.organization_id}
				/>
			)}
			<ChatProjectDialog
				key={isEditing ? project.id : "closed"}
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
