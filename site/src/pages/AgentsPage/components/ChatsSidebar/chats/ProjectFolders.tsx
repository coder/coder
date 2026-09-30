import { cn } from "cn";
import {
	ChevronRightIcon,
	EllipsisVerticalIcon,
	PlusIcon,
	SquarePenIcon,
} from "lucide-react";
import { useRef } from "react";
import { Link, NavLink, type To, useLocation } from "react-router";
import type { Chat, ChatProject } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
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
import {
	rowActionsTriggerProps,
	stopRowContextMenu,
} from "../tree/rowMenuEvents";
import { SidebarSectionHeader } from "./SidebarSectionHeader";

export type ProjectDialogMode =
	| { mode: "create" }
	| { mode: "edit"; project: ChatProject };

/** `trigger` receives focus when the dialog closes. */
export type OpenProjectDialog = (
	mode: ProjectDialogMode,
	trigger?: HTMLElement | null,
) => void;
export type DeleteProject = (
	project: ChatProject,
	trigger?: HTMLElement | null,
) => void;

type ProjectFoldersProps = {
	readonly projects: readonly ChatProject[];
	readonly chatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	readonly expandedProjectIds: Readonly<Record<string, boolean>>;
	readonly onToggle: (projectId: string) => void;
	readonly onOpenProjectDialog: OpenProjectDialog;
	readonly onDelete: DeleteProject;
	readonly isLoading?: boolean;
	readonly error?: unknown;
	readonly onRetry: () => void;
	readonly emptyMessage: string;
};

export const ProjectFolders: React.FC<ProjectFoldersProps> = ({
	projects,
	chatsByProjectId,
	expandedProjectIds,
	onToggle,
	onOpenProjectDialog,
	onDelete,
	isLoading = false,
	error,
	onRetry,
	emptyMessage,
}) => {
	const location = useLocation();

	return (
		// Bounded so a long project list cannot push the Chats section off
		// screen; the folders scroll on their own past that point.
		<div className="flex max-h-[40vh] shrink-0 flex-col">
			<SidebarSectionHeader
				title="Projects"
				actions={
					<Button
						variant="subtle"
						size="icon"
						className="size-7"
						aria-label="New project"
						onClick={() => onOpenProjectDialog({ mode: "create" })}
					>
						<PlusIcon />
					</Button>
				}
			/>
			<div className="min-h-0 overflow-y-auto px-2">
				{Boolean(error) && (
					<ErrorAlert
						error={error}
						className="mb-1"
						actions={
							<Button size="sm" variant="outline" onClick={onRetry}>
								Retry
							</Button>
						}
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
								onEdit={(trigger) =>
									onOpenProjectDialog({ mode: "edit", project }, trigger)
								}
								onDelete={(trigger) => onDelete(project, trigger)}
								emptyMessage={emptyMessage}
							/>
						))}
					</div>
				)}
			</div>
		</div>
	);
};

type ProjectFolderProps = {
	readonly project: ChatProject;
	readonly chats: readonly Chat[];
	readonly expanded: boolean;
	readonly locationSearch: string;
	readonly onToggle: () => void;
	readonly onEdit: (trigger: HTMLElement | null) => void;
	readonly onDelete: (trigger: HTMLElement | null) => void;
	readonly emptyMessage: string;
};

const ProjectFolder: React.FC<ProjectFolderProps> = ({
	project,
	chats,
	expanded,
	locationSearch,
	onToggle,
	onEdit,
	onDelete,
	emptyMessage,
}) => {
	const projectPath: To = {
		pathname: buildAgentProjectPath(project.id),
		search: locationSearch,
	};
	// Menu items unmount on select, so dialogs return focus to the control
	// that opened the menu: the actions button, or the project link for the
	// context menu.
	const projectLinkRef = useRef<HTMLAnchorElement>(null);
	const projectActionsButtonRef = useRef<HTMLButtonElement>(null);

	return (
		<div>
			<ContextMenu>
				<ContextMenuTrigger asChild>
					<div className="group relative flex items-center gap-1 rounded-md pl-1 pr-2 text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary has-[[aria-current=page]]:bg-surface-quaternary/50 has-[[aria-current=page]]:text-content-primary">
						<Button
							variant="subtle"
							size="icon"
							className="size-6 min-w-0 p-0 text-current [&>svg]:size-3.5"
							aria-expanded={expanded}
							aria-label={`${expanded ? "Collapse" : "Expand"} ${project.name}`}
							onClick={onToggle}
						>
							<ChevronRightIcon
								aria-hidden="true"
								className={cn("transition-transform", expanded && "rotate-90")}
							/>
						</Button>
						<NavLink
							ref={projectLinkRef}
							to={projectPath}
							className="flex min-w-0 flex-1 items-center gap-2 py-1 text-[13px] text-content-primary no-underline"
						>
							<ChatProjectIcon
								project={project}
								expanded={expanded}
								className="size-4"
							/>
							<span className="flex-1 truncate">{project.name}</span>
						</NavLink>
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									ref={projectActionsButtonRef}
									variant="subtle"
									size="icon"
									className="size-6 opacity-0 group-hover:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100"
									aria-label={`Open project actions for ${project.name}`}
									{...rowActionsTriggerProps}
								>
									<EllipsisVerticalIcon />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent
								align="end"
								onContextMenu={stopRowContextMenu}
							>
								<ProjectFolderMenuItems
									Item={DropdownMenuItem}
									Separator={DropdownMenuSeparator}
									projectPath={projectPath}
									onEdit={() => onEdit(projectActionsButtonRef.current)}
									onDelete={() => onDelete(projectActionsButtonRef.current)}
								/>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</ContextMenuTrigger>
				<ContextMenuContent>
					<ProjectFolderMenuItems
						Item={ContextMenuItem}
						Separator={ContextMenuSeparator}
						projectPath={projectPath}
						onEdit={() => onEdit(projectLinkRef.current)}
						onDelete={() => onDelete(projectLinkRef.current)}
					/>
				</ContextMenuContent>
			</ContextMenu>
			{expanded && (
				// pl-[7px] and pr-2 offset ChatTreeNode's -mx-2 bleed: rows span the
				// folder row's width and an active row's border covers the guide line.
				<div className="ml-3 flex flex-col gap-0.5 border-0 border-l border-solid border-border-default pl-[7px] pr-2">
					{chats.length === 0 ? (
						<p className="m-0 px-2 py-1 text-xs text-content-secondary">
							{emptyMessage}
						</p>
					) : (
						chats.map((chat) => <ChatTreeNode key={chat.id} chat={chat} />)
					)}
				</div>
			)}
		</div>
	);
};

type ProjectFolderMenuItem = typeof DropdownMenuItem | typeof ContextMenuItem;
type ProjectFolderMenuSeparator =
	| typeof DropdownMenuSeparator
	| typeof ContextMenuSeparator;

type ProjectFolderMenuItemsProps = {
	readonly Item: ProjectFolderMenuItem;
	readonly Separator: ProjectFolderMenuSeparator;
	readonly projectPath: To;
	readonly onEdit: () => void;
	readonly onDelete: () => void;
};

const ProjectFolderMenuItems: React.FC<ProjectFolderMenuItemsProps> = ({
	Item,
	Separator,
	projectPath,
	onEdit,
	onDelete,
}) => (
	<>
		<Item asChild>
			<Link to={projectPath}>
				<SquarePenIcon />
				New chat
			</Link>
		</Item>
		<Separator />
		<Item onSelect={onEdit}>Edit project</Item>
		<Item
			className="text-content-destructive focus:text-content-destructive"
			onSelect={onDelete}
		>
			Delete project
		</Item>
	</>
);
