import { EllipsisVerticalIcon, Trash2Icon } from "lucide-react";
import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useLocation, useNavigate } from "react-router";
import { getErrorMessage } from "#/api/errors";
import { deleteChatProject } from "#/api/queries/chatProjects";
import type { ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { toast } from "#/components/Toaster/toast";
import { draftStorageKeys } from "../AgentCreateForm";
import { normalizeLocationSearch } from "../ChatsSidebar/locationSearch";
import { deleteAgentInfo } from "../chatProjectCopy";

type ProjectActionsMenuProps = {
	readonly project: ChatProject;
};

/** The agent's actions menu. Deleting asks for confirmation first. */
export const ProjectActionsMenu: React.FC<ProjectActionsMenuProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const location = useLocation();
	// The project list refetch after a delete unmounts this page, so the
	// toast and navigation run on the mutation itself rather than per call.
	const deleteMutation = useMutation({
		...deleteChatProject(queryClient),
		onSuccess: (_data, deletedProject) => {
			// Nothing can open the deleted project's composer again.
			const draftKeys = draftStorageKeys(deletedProject.id);
			localStorage.removeItem(draftKeys.text);
			localStorage.removeItem(draftKeys.attachments);
			toast.success("Agent deleted");
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
			toast.error(getErrorMessage(error, "Failed to delete agent."));
		},
	});
	const [isDeleteOpen, setIsDeleteOpen] = useState(false);
	const triggerRef = useRef<HTMLButtonElement>(null);

	if (!project.permissions.delete) {
		return null;
	}

	return (
		<>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						ref={triggerRef}
						variant="subtle"
						size="icon"
						aria-label="Agent actions"
					>
						<EllipsisVerticalIcon />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem
						className="text-content-destructive focus:text-content-destructive"
						onSelect={() => setIsDeleteOpen(true)}
					>
						<Trash2Icon />
						Delete agent
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
			<DeleteDialog
				isOpen={isDeleteOpen}
				onConfirm={() => deleteMutation.mutate(project)}
				onCancel={() => {
					setIsDeleteOpen(false);
					// The menu item unmounts on select, so focus returns to the
					// trigger instead.
					requestAnimationFrame(() => triggerRef.current?.focus());
				}}
				entity="agent"
				name={project.name}
				confirmLoading={deleteMutation.isPending}
				info={deleteAgentInfo}
			/>
		</>
	);
};
