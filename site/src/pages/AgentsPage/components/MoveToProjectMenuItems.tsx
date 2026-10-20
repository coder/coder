import { CheckIcon, FolderInputIcon, FolderMinusIcon } from "lucide-react";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { chatProjects } from "#/api/queries/chatProjects";
import { moveChatToProject } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import type {
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuSub,
	ContextMenuSubContent,
	ContextMenuSubTrigger,
} from "#/components/ContextMenu/ContextMenu";
import type {
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { toast } from "#/components/Toaster/toast";
import { ChatProjectIcon } from "./ChatProjectIcon";

/**
 * Moves a chat and reports a failure. The error handling is a mutation
 * option, so the rollback and the toast still run after the menu unmounts.
 */
export const useMoveChatToProject = () => {
	const queryClient = useQueryClient();
	const options = moveChatToProject(queryClient);
	return useMutation({
		...options,
		onError: (error, variables, context) => {
			options.onError(error, variables, context);
			toast.error(getErrorMessage(error, "Failed to move chat."));
		},
	});
};

type MoveToProjectMenuItemsProps = {
	readonly chat: Chat;
	/**
	 * Turns the submenu off while the chat or a subagent is in a state that
	 * blocks archiving.
	 */
	readonly blocked: boolean;
	/** The ID of the hint that explains why the submenu is off. */
	readonly describedBy?: string;
	readonly Item: typeof DropdownMenuItem | typeof ContextMenuItem;
	readonly Separator:
		| typeof DropdownMenuSeparator
		| typeof ContextMenuSeparator;
	readonly Sub: typeof DropdownMenuSub | typeof ContextMenuSub;
	readonly SubTrigger:
		| typeof DropdownMenuSubTrigger
		| typeof ContextMenuSubTrigger;
	readonly SubContent:
		| typeof DropdownMenuSubContent
		| typeof ContextMenuSubContent;
};

/** Moves a chat into a project of its organization, or out of its project. */
export const MoveToProjectMenuItems: React.FC<MoveToProjectMenuItemsProps> = ({
	chat,
	blocked,
	describedBy,
	Item,
	Separator,
	Sub,
	SubTrigger,
	SubContent,
}) => {
	const [open, setOpen] = useState(false);
	// The sidebar keeps the list current and the project mutations invalidate
	// it, so opening the menu sends no request.
	const projectsQuery = useQuery({
		...chatProjects(),
		staleTime: Number.POSITIVE_INFINITY,
	});
	const projects = (projectsQuery.data ?? []).filter(
		(project) => project.organization_id === chat.organization_id,
	);
	const mutation = useMoveChatToProject();
	const move = (projectId: string | null) =>
		mutation.mutate({ chatId: chat.id, projectId });

	return (
		<Sub open={open} onOpenChange={(next) => setOpen(next && !blocked)}>
			{/* Radix keeps disabled items out of keyboard focus, so a blocked
			    trigger only looks disabled and keeps its hint reachable. */}
			<SubTrigger
				aria-disabled={blocked || undefined}
				data-disabled={blocked ? "" : undefined}
				aria-describedby={describedBy}
			>
				<FolderInputIcon />
				Move to project
			</SubTrigger>
			<SubContent className="max-h-80 max-w-72 overflow-y-auto">
				{projectsQuery.isPending && <Item disabled>Loading projects…</Item>}
				{projectsQuery.isError && (
					<Item disabled>
						{getErrorMessage(projectsQuery.error, "Failed to load projects.")}
					</Item>
				)}
				{projectsQuery.isSuccess && projects.length === 0 && (
					<Item disabled>No projects</Item>
				)}
				{projects.map((project) => {
					const isCurrent = project.id === chat.project_id;
					// The current project stays enabled because Radix keeps disabled
					// items out of keyboard focus, so keyboard and screen reader users
					// could not reach it.
					return (
						<Item
							key={project.id}
							aria-current={isCurrent ? "true" : undefined}
							onSelect={isCurrent ? undefined : () => move(project.id)}
						>
							<ChatProjectIcon project={project} />
							<span className="truncate">{project.name}</span>
							{isCurrent && <CheckIcon className="ml-auto" />}
						</Item>
					);
				})}
				{chat.project_id && (
					<>
						<Separator />
						<Item onSelect={() => move(null)}>
							<FolderMinusIcon />
							Remove from project
						</Item>
					</>
				)}
			</SubContent>
		</Sub>
	);
};
