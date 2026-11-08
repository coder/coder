import { EllipsisVerticalIcon } from "lucide-react";
import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useLocation, useNavigate } from "react-router";
import { getErrorMessage } from "#/api/errors";
import {
	deleteChatProject,
	updateChatProject,
} from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { toast } from "#/components/Toaster/toast";
import { draftStorageKeys } from "../AgentCreateForm";
import {
	hasProjectUpdateOrDeleteAction,
	ProjectActionsMenuItems,
} from "../ChatsSidebar/chats/ProjectActionsMenuItems";
import { ChatProjectDialog } from "../ChatsSidebar/dialogs/ChatProjectDialog";
import { normalizeLocationSearch } from "../ChatsSidebar/locationSearch";
import { ProjectPageHeader } from "./ProjectPageHeader";

type ProjectHeaderSectionProps = {
	readonly project: ChatProject;
};

/** The project page header with its edit and delete actions. */
export const ProjectHeaderSection: React.FC<ProjectHeaderSectionProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const location = useLocation();

	const updateMutation = useMutation(updateChatProject(queryClient));
	// The project list refetch after a delete unmounts this page, so the
	// toast and navigation run on the mutation itself rather than per call.
	const deleteMutation = useMutation({
		...deleteChatProject(queryClient),
		onSuccess: (_data, deletedProject) => {
			// Nothing can open the deleted project's composer again.
			const draftKeys = draftStorageKeys(deletedProject.id);
			localStorage.removeItem(draftKeys.text);
			localStorage.removeItem(draftKeys.attachments);
			toast.success("Project deleted");
			// Keeps the sidebar filters, which live in the query string.
			navigate(
				{
					pathname: "/agents",
					search: normalizeLocationSearch(location.search),
				},
				{ replace: true },
			);
		},
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to delete project."));
		},
	});

	const [isEditOpen, setIsEditOpen] = useState(false);
	const [isDeleteOpen, setIsDeleteOpen] = useState(false);
	// Menu items unmount on select, so the dialogs return focus to the menu
	// trigger instead.
	const actionsButtonRef = useRef<HTMLButtonElement>(null);
	const restoreFocus = () => {
		requestAnimationFrame(() => actionsButtonRef.current?.focus());
	};
	const closeEditDialog = () => {
		setIsEditOpen(false);
		restoreFocus();
	};
	const closeDeleteDialog = () => {
		setIsDeleteOpen(false);
		restoreFocus();
	};

	return (
		<>
			<ProjectPageHeader
				project={project}
				actions={
					hasProjectUpdateOrDeleteAction(project.permissions) && (
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									ref={actionsButtonRef}
									variant="subtle"
									size="icon"
									aria-label="Project actions"
								>
									<EllipsisVerticalIcon />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<ProjectActionsMenuItems
									Item={DropdownMenuItem}
									Separator={DropdownMenuSeparator}
									permissions={project.permissions}
									onEdit={() => {
										updateMutation.reset();
										setIsEditOpen(true);
									}}
									onDelete={() => setIsDeleteOpen(true)}
								/>
							</DropdownMenuContent>
						</DropdownMenu>
					)
				}
			/>
			<ChatProjectDialog
				project={project}
				open={isEditOpen}
				onOpenChange={(open) => {
					if (!open) closeEditDialog();
				}}
				isSubmitting={updateMutation.isPending}
				error={updateMutation.error}
				onSubmit={async (request) => {
					await updateMutation.mutateAsync({ project, request });
					closeEditDialog();
				}}
			/>
			<DeleteDialog
				isOpen={isDeleteOpen}
				onConfirm={() => deleteMutation.mutate(project)}
				onCancel={closeDeleteDialog}
				entity="project"
				name={project.name}
				confirmLoading={deleteMutation.isPending}
				info="Its memories and instructions will be deleted. Chats in it, including chats started by people it is shared with, will be kept and move back to each owner's Chats list."
			/>
		</>
	);
};
