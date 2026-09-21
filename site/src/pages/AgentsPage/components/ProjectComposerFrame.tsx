import { PencilIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorMessage } from "#/api/errors";
import {
	chatProjectPermissions,
	updateChatProject,
} from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { chatProjectPermissionsFor } from "#/modules/permissions/chatProjects";
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

/** Edit control under the composer for users allowed to update the project. */
export const ProjectComposerFooter: FC<ProjectComposerFooterProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const [isEditing, setIsEditing] = useState(false);
	const permissionsQuery = useQuery(chatProjectPermissions([project]));
	const updateProjectMutation = useMutation(updateChatProject(queryClient));
	const { canUpdate } = chatProjectPermissionsFor(
		project,
		permissionsQuery.data,
	);

	// Without any permission data the edit control cannot be decided, so
	// offer a retry instead of silently hiding it. Cached data keeps the
	// control through a failed background refetch.
	if (permissionsQuery.data === undefined && permissionsQuery.error) {
		return (
			<div className="flex items-center justify-center gap-2 pt-2 text-xs text-content-destructive">
				<span>
					{getErrorMessage(
						permissionsQuery.error,
						"Failed to load project permissions.",
					)}
				</span>
				<Button
					size="sm"
					variant="outline"
					onClick={() => void permissionsQuery.refetch()}
				>
					Retry
				</Button>
			</div>
		);
	}

	if (!canUpdate) {
		return null;
	}

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
