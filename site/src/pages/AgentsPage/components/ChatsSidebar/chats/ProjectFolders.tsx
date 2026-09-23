import { cn } from "cn";
import {
	ChevronRightIcon,
	EllipsisVerticalIcon,
	PlusIcon,
	SquarePenIcon,
} from "lucide-react";
import type { FC } from "react";
import { Link, NavLink, useLocation } from "react-router";
import { getErrorMessage } from "#/api/errors";
import type { Chat, ChatProject } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuTrigger,
} from "#/components/ContextMenu/ContextMenu";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { buildAgentProjectPath } from "../../../utils/navigation";
import { ChatProjectIcon } from "../../ChatProjectIcon";
import { ChatTreeNode } from "../tree/ChatTreeNode";

const getProjectFolderToggleTestId = (projectId: string) =>
	`agents-project-folder-toggle-${projectId}`;

type ProjectFoldersProps = {
	readonly projects: readonly ChatProject[];
	readonly chatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	readonly expandedProjectIds: Readonly<Record<string, boolean>>;
	readonly onToggle: (projectId: string) => void;
	readonly onCreate: () => void;
	readonly onEdit: (project: ChatProject) => void;
	readonly onDelete: (project: ChatProject) => void;
	readonly isLoading?: boolean;
	readonly error?: unknown;
	readonly onRetry: () => void;
};

/**
 * Projects section above Chats, with a matching header. A folder holds the
 * user's chats in that project; those chats are omitted from the time
 * sections below so each chat appears once.
 */
export const ProjectFolders: FC<ProjectFoldersProps> = ({
	projects,
	chatsByProjectId,
	expandedProjectIds,
	onToggle,
	onCreate,
	onEdit,
	onDelete,
	isLoading = false,
	error,
	onRetry,
}) => {
	const location = useLocation();

	return (
		// Bounded so a long project list cannot push the Chats section off
		// screen; the folders scroll on their own past that point.
		<div className="flex max-h-[40vh] shrink-0 flex-col">
			<div className="mx-2 pt-6 mb-1.5">
				<div className="ml-2.5 mr-2 flex h-7 items-center justify-between">
					<h2 className="m-0 text-sm font-normal leading-6 text-content-secondary">
						Projects
					</h2>
					<Button
						variant="subtle"
						size="icon"
						className="size-7"
						aria-label="New project"
						onClick={onCreate}
					>
						<PlusIcon />
					</Button>
				</div>
			</div>
			<div className="min-h-0 overflow-y-auto px-2">
				{Boolean(error) && (
					<FolderError
						message={getErrorMessage(error, "Failed to load projects.")}
						onRetry={onRetry}
					/>
				)}
				{isLoading && <Skeleton className="ml-2.5 h-3.5 w-20" />}
				{projects.length > 0 && (
					<div className="flex flex-col gap-0.5">
						{projects.map((project) => (
							<ProjectFolder
								key={project.id}
								project={project}
								chats={chatsByProjectId.get(project.id) ?? []}
								expanded={Boolean(expandedProjectIds[project.id])}
								locationSearch={location.search}
								onToggle={() => onToggle(project.id)}
								onEdit={() => onEdit(project)}
								onDelete={() => onDelete(project)}
							/>
						))}
					</div>
				)}
			</div>
		</div>
	);
};

const FolderError: FC<{
	readonly message: string;
	readonly onRetry: () => void;
}> = ({ message, onRetry }) => (
	<div className="mb-1 flex items-center justify-between gap-2 px-2 py-1 text-xs text-content-destructive">
		<span>{message}</span>
		<Button size="sm" variant="outline" onClick={onRetry}>
			Retry
		</Button>
	</div>
);

type ProjectFolderProps = {
	readonly project: ChatProject;
	readonly chats: readonly Chat[];
	readonly expanded: boolean;
	readonly locationSearch: string;
	readonly onToggle: () => void;
	readonly onEdit: () => void;
	readonly onDelete: () => void;
};

const ProjectFolder: FC<ProjectFolderProps> = ({
	project,
	chats,
	expanded,
	locationSearch,
	onToggle,
	onEdit,
	onDelete,
}) => {
	const projectPath = {
		pathname: buildAgentProjectPath(project.id),
		search: locationSearch,
	};
	const row = (
		<div className="group relative flex items-center gap-1 rounded-md pl-1 pr-2 text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary has-[[aria-current=page]]:bg-surface-quaternary/50 has-[[aria-current=page]]:text-content-primary">
			<button
				type="button"
				className="flex size-6 shrink-0 cursor-pointer appearance-none items-center justify-center rounded border-0 bg-transparent p-0 text-current focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
				aria-expanded={expanded}
				aria-label={`${expanded ? "Collapse" : "Expand"} ${project.name}`}
				data-testid={getProjectFolderToggleTestId(project.id)}
				onClick={onToggle}
			>
				<ChevronRightIcon
					aria-hidden="true"
					className={cn(
						"size-3.5 transition-transform",
						expanded && "rotate-90",
					)}
				/>
			</button>
			<NavLink
				to={projectPath}
				className="flex min-w-0 flex-1 items-center gap-2 py-1 text-[13px] text-content-primary no-underline"
			>
				<ChatProjectIcon
					project={project}
					expanded={expanded}
					className="size-4"
				/>
				<span className="min-w-0 flex-1 truncate">{project.name}</span>
			</NavLink>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						variant="subtle"
						size="icon"
						className="size-6 min-w-0 opacity-0 group-hover:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100"
						aria-label={`Open project actions for ${project.name}`}
					>
						<EllipsisVerticalIcon className="size-3.5" />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem asChild>
						<Link to={projectPath}>
							<SquarePenIcon />
							New chat
						</Link>
					</DropdownMenuItem>
					<DropdownMenuSeparator />
					<DropdownMenuItem onSelect={onEdit}>Edit project</DropdownMenuItem>
					<DropdownMenuItem
						className="text-content-destructive focus:text-content-destructive"
						onSelect={onDelete}
					>
						Delete project
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		</div>
	);

	return (
		<div>
			<ContextMenu>
				<ContextMenuTrigger asChild>{row}</ContextMenuTrigger>
				<ContextMenuContent>
					<ContextMenuItem asChild>
						<Link to={projectPath}>
							<SquarePenIcon />
							New chat
						</Link>
					</ContextMenuItem>
					<ContextMenuSeparator />
					<ContextMenuItem onSelect={onEdit}>Edit project</ContextMenuItem>
					<ContextMenuItem
						className="text-content-destructive focus:text-content-destructive"
						onSelect={onDelete}
					>
						Delete project
					</ContextMenuItem>
				</ContextMenuContent>
			</ContextMenu>
			{expanded && (
				<div className="ml-3 flex flex-col gap-0.5 border-0 border-l border-solid border-border-default pl-1">
					{chats.length === 0 ? (
						<p className="m-0 px-2 py-1 text-xs text-content-secondary">
							No chats yet
						</p>
					) : (
						chats.map((chat) => <ChatTreeNode key={chat.id} chat={chat} />)
					)}
				</div>
			)}
		</div>
	);
};
