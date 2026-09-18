import { cn } from "cn";
import { ChevronDownIcon, PencilIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatProjectPermissions,
	updateChatProject,
} from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { chatProjectPermissionsFor } from "#/modules/permissions/chatProjects";
import { ChatProjectDialog } from "./ChatsSidebar/dialogs/ChatProjectDialog";
import { MemorySection } from "./MemorySection";

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
 * Project controls that live under the composer: edit for users allowed to
 * update the project and a collapsed view of the shared memory the agent
 * maintains for it.
 */
export const ProjectComposerFooter: FC<ProjectComposerFooterProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const [isEditing, setIsEditing] = useState(false);
	const [isMemoryOpen, setIsMemoryOpen] = useState(false);
	const permissionsQuery = useQuery(chatProjectPermissions([project]));
	const updateProjectMutation = useMutation(updateChatProject(queryClient));
	const { canUpdate } = chatProjectPermissionsFor(
		project,
		permissionsQuery.data,
	);

	return (
		<Collapsible open={isMemoryOpen} onOpenChange={setIsMemoryOpen}>
			<div className="flex items-center justify-center gap-1 pt-2">
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
				<CollapsibleTrigger asChild>
					<Button variant="subtle" size="sm" className="text-content-secondary">
						Memory
						<ChevronDownIcon
							className={cn(
								"transition-transform",
								isMemoryOpen && "rotate-180",
							)}
						/>
					</Button>
				</CollapsibleTrigger>
			</div>
			<CollapsibleContent>
				<MemorySection
					scope={{ kind: "project", projectId: project.id }}
					className="mt-4 border-t-0 pt-0"
				/>
			</CollapsibleContent>
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
		</Collapsible>
	);
};
