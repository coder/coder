import { cn } from "cn";
import { ChevronRightIcon, EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import { useState } from "react";
import { NavLink, type To, useLocation } from "react-router";
import type { Chat, ChatProject, Organization } from "#/api/typesGenerated";
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
import { ProjectActionsMenuItems } from "./ProjectActionsMenuItems";
import { getOrganizationLabels } from "./projectGrouping";
import { SidebarSectionHeader } from "./SidebarSectionHeader";

export type ProjectDialogState =
	| { mode: "create" }
	| { mode: "edit"; project: ChatProject };

export type OpenProjectDialog = (dialog: ProjectDialogState) => void;
export type DeleteProject = (project: ChatProject) => void;

type ProjectFoldersProps = {
	readonly projects: readonly ChatProject[];
	readonly organizations: readonly Organization[];
	readonly chatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	/** Projects with pinned chats, which the Pinned section lists instead. */
	readonly projectIdsWithPinnedChats: ReadonlySet<string>;
	readonly expandedProjectIds: Readonly<Record<string, boolean>>;
	readonly onToggle: (projectId: string) => void;
	readonly onOpenProjectDialog: OpenProjectDialog;
	readonly onDelete: DeleteProject;
	readonly isLoading: boolean;
	readonly error: unknown;
	readonly onRetry: () => void;
	readonly emptyMessage: string;
};

export const ProjectFolders: React.FC<ProjectFoldersProps> = ({
	projects,
	organizations,
	chatsByProjectId,
	projectIdsWithPinnedChats,
	expandedProjectIds,
	onToggle,
	onOpenProjectDialog,
	onDelete,
	isLoading,
	error,
	onRetry,
	emptyMessage,
}) => {
	const location = useLocation();
	const organizationLabels = getOrganizationLabels(projects, organizations);
	const [isExpanded, setIsExpanded] = useState(true);

	return (
		<div>
			<SidebarSectionHeader
				title="Projects"
				collapsible={{
					expanded: isExpanded,
					onToggle: () => setIsExpanded((expanded) => !expanded),
				}}
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
			{isExpanded && (
				<div className="px-2">
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
					{!isLoading && !error && projects.length === 0 && (
						<p className="m-0 px-2.5 py-1 text-xs text-content-secondary">
							No projects yet
						</p>
					)}
					{projects.length > 0 && (
						<div className="flex flex-col gap-0.5">
							{projects.map((project) => (
								<ProjectFolder
									key={project.id}
									project={project}
									organizationLabel={organizationLabels.get(project.id)}
									chats={chatsByProjectId.get(project.id) ?? []}
									expanded={Boolean(expandedProjectIds[project.id])}
									locationSearch={location.search}
									onToggle={() => onToggle(project.id)}
									onEdit={() => onOpenProjectDialog({ mode: "edit", project })}
									onDelete={() => onDelete(project)}
									emptyMessage={
										projectIdsWithPinnedChats.has(project.id)
											? "All agents are pinned"
											: emptyMessage
									}
								/>
							))}
						</div>
					)}
				</div>
			)}
		</div>
	);
};

type ProjectFolderProps = {
	readonly project: ChatProject;
	readonly organizationLabel: string | undefined;
	readonly chats: readonly Chat[];
	readonly expanded: boolean;
	readonly locationSearch: string;
	readonly onToggle: () => void;
	readonly onEdit: () => void;
	readonly onDelete: () => void;
	readonly emptyMessage: string;
};

const ProjectFolder: React.FC<ProjectFolderProps> = ({
	project,
	organizationLabel,
	chats,
	expanded,
	locationSearch,
	onToggle,
	onEdit,
	onDelete,
	emptyMessage,
}) => {
	const projectPath: To = {
		pathname: buildAgentProjectPath({ projectId: project.id }),
		search: locationSearch,
	};
	const projectLabel = organizationLabel
		? `${project.name} (${organizationLabel})`
		: project.name;
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
							aria-label={`${expanded ? "Collapse" : "Expand"} ${projectLabel}`}
							onClick={onToggle}
						>
							<ChevronRightIcon
								aria-hidden="true"
								className={cn("transition-transform", expanded && "rotate-90")}
							/>
						</Button>
						<NavLink
							to={projectPath}
							aria-label={projectLabel}
							className="flex min-w-0 flex-1 items-center gap-2 py-1 text-[13px] text-content-primary no-underline"
						>
							<ChatProjectIcon
								project={project}
								expanded={expanded}
								className="size-4"
							/>
							<span className="flex-1 truncate">{project.name}</span>
							{organizationLabel && (
								<span className="max-w-[40%] shrink-0 truncate text-xs text-content-secondary">
									{organizationLabel}
								</span>
							)}
						</NavLink>
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									variant="subtle"
									size="icon"
									className="size-6 opacity-0 group-hover:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100"
									aria-label={`Open project actions for ${projectLabel}`}
									// Keeps a right-click on this button from also
									// opening the row's context menu.
									onContextMenuCapture={(e) => {
										e.preventDefault();
										e.stopPropagation();
									}}
									onMouseDownCapture={(e) => {
										if (e.button === 2) {
											e.preventDefault();
											e.stopPropagation();
										}
									}}
									onPointerDownCapture={(e) => {
										if (e.button === 2) {
											e.preventDefault();
											e.stopPropagation();
										}
									}}
								>
									<EllipsisVerticalIcon />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent
								align="end"
								// Portaled, but React events still bubble to the row's
								// context-menu trigger and would open a second menu.
								onContextMenu={(e) => {
									e.preventDefault();
									e.stopPropagation();
								}}
							>
								<ProjectActionsMenuItems
									Item={DropdownMenuItem}
									Separator={DropdownMenuSeparator}
									projectPath={projectPath}
									onEdit={onEdit}
									onDelete={onDelete}
								/>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</ContextMenuTrigger>
				<ContextMenuContent>
					<ProjectActionsMenuItems
						Item={ContextMenuItem}
						Separator={ContextMenuSeparator}
						projectPath={projectPath}
						onEdit={onEdit}
						onDelete={onDelete}
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
