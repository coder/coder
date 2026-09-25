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
import { shortRelativeTime } from "#/utils/time";
import { buildAgentProjectPath } from "../../../utils/navigation";
import { ChatTreeNode } from "../tree/ChatTreeNode";
import { ProjectIcon } from "./ProjectIcon";
import { SidebarGroupHeading } from "./SidebarGroupHeading";

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
	/** True when sidebar filters or the archived view may be hiding chats. */
	readonly isFiltered: boolean;
	readonly error?: unknown;
	readonly onRetry: () => void;
};

/**
 * Projects as folders at the top of the sidebar. A folder holds the user's
 * chats in that project; those chats are omitted from the time sections below
 * so each chat appears once.
 */
export const ProjectFolders: FC<ProjectFoldersProps> = ({
	projects,
	chatsByProjectId,
	expandedProjectIds,
	onToggle,
	onCreate,
	onEdit,
	onDelete,
	isFiltered,
	error,
	onRetry,
}) => {
	const location = useLocation();

	return (
		<div className="mb-4">
			<SidebarGroupHeading
				label="Projects"
				action={
					<Button
						variant="subtle"
						size="icon"
						className="size-7 min-w-0 text-content-secondary hover:text-content-primary"
						aria-label="New project"
						onClick={onCreate}
					>
						<PlusIcon className="size-3.5" />
					</Button>
				}
			/>
			{Boolean(error) && (
				<FolderError
					message={getErrorMessage(error, "Failed to load projects.")}
					onRetry={onRetry}
				/>
			)}
			{projects.length > 0 ? (
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
							isFiltered={isFiltered}
						/>
					))}
				</div>
			) : (
				!error && (
					<button
						type="button"
						className="flex w-full cursor-pointer items-center justify-center gap-1.5 rounded-lg border border-dashed border-border-default bg-transparent px-3 py-3.5 font-sans text-[13px] text-content-primary hover:bg-surface-tertiary/50 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
						onClick={onCreate}
					>
						<PlusIcon aria-hidden="true" className="size-3.5" />
						Add project
					</button>
				)
			)}
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
	readonly isFiltered: boolean;
};

/** Most recent activity in the folder: its newest chat, else the project. */
const getFolderActivityAt = (
	project: ChatProject,
	chats: readonly Chat[],
): string => {
	let latest = project.updated_at;
	for (const chat of chats) {
		if (chat.updated_at > latest) {
			latest = chat.updated_at;
		}
	}
	return latest;
};

const ProjectFolder: FC<ProjectFolderProps> = ({
	project,
	chats,
	expanded,
	locationSearch,
	onToggle,
	onEdit,
	onDelete,
	isFiltered,
}) => {
	const projectPath = {
		pathname: buildAgentProjectPath(project.id),
		search: locationSearch,
	};
	const row = (
		<div className="group relative flex h-8 items-center gap-1.5 rounded-md pl-1 pr-1.5 text-content-secondary has-data-[state=open]:bg-surface-tertiary hover:bg-surface-tertiary/50 hover:text-content-primary has-[[aria-current=page]]:bg-surface-quaternary/50 has-[[aria-current=page]]:text-content-primary">
			<button
				type="button"
				className="flex size-5 shrink-0 cursor-pointer appearance-none items-center justify-center rounded border-0 bg-transparent p-0 text-current focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
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
				className="flex h-full min-w-0 flex-1 items-center gap-2 text-[13px] text-content-primary no-underline"
			>
				<ProjectIcon icon={project.icon} />
				<span className="min-w-0 flex-1 truncate">{project.name}</span>
			</NavLink>
			<div className="relative flex h-6 w-7 shrink-0 items-center justify-end">
				<span
					data-pixel="ignore"
					className="text-xs tabular-nums text-content-secondary/50 group-has-data-[state=open]:hidden [@media(hover:hover)]:group-hover:hidden"
				>
					{shortRelativeTime(getFolderActivityAt(project, chats))}
				</span>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							className="absolute inset-0 flex h-6 w-7 min-w-0 justify-end rounded-none px-0 text-content-secondary opacity-0 hover:text-content-primary focus-visible:opacity-100 data-[state=open]:opacity-100 [@media(hover:hover)]:group-hover:opacity-100"
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
				// The guide line sits under the chevron's center (4px row
				// padding plus half of the 20px chevron slot).
				<div className="ml-3.5 mt-0.5 flex flex-col gap-0.5 border-0 border-l border-solid border-border-default pl-1.5">
					{chats.length === 0 ? (
						<p className="m-0 px-2 py-1.5 text-xs text-content-secondary">
							{isFiltered ? "No matches for this filter" : "No chats yet"}
						</p>
					) : (
						chats.map((chat) => <ChatTreeNode key={chat.id} chat={chat} />)
					)}
				</div>
			)}
		</div>
	);
};
