import { cn } from "cn";
import { ChevronRightIcon, EllipsisVerticalIcon } from "lucide-react";
import { useRef } from "react";
import { useMutation, useQueryClient } from "react-query";
import { NavLink, type To, useLocation, useMatch } from "react-router";
import { getErrorMessage } from "#/api/errors";
import { unpinChatProject } from "#/api/queries/chatProjects";
import type { ChatListInput } from "#/api/queries/chats";
import type {
	Chat,
	ChatProject,
	ChatProjectSummary,
	Organization,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuTrigger,
	preventRowContextMenuProps,
} from "#/components/ContextMenu/ContextMenu";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { toast } from "#/components/Toaster/toast";
import { buildAgentProjectPath } from "../../../utils/navigation";
import { ChatProjectIcon } from "../../ChatProjectIcon";
import { ChatDropArea } from "./ChatDropArea";
import { ProjectActionsMenuItems } from "./ProjectActionsMenuItems";
import { ProjectFolderChats } from "./ProjectFolderChats";
import { getOrganizationLabels } from "./projectGrouping";

/** Acts on one project, such as editing or deleting it. */
export type ProjectAction = (project: ChatProject) => void;

export type ProjectFoldersProps = {
	/** The user's pinned projects, in pin order. */
	readonly pinnedProjects: readonly ChatProjectSummary[];
	readonly organizations: readonly Organization[];
	readonly chatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	/** The Pinned section lists these, so folders only count them. */
	readonly pinnedChatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	readonly chatListInput: ChatListInput;
	readonly expandedProjectIds: Readonly<Record<string, boolean>>;
	readonly onToggle: (projectId: string) => void;
	readonly onEdit: ProjectAction;
	readonly onDelete: ProjectAction;
	readonly isLoading: boolean;
	readonly error: unknown;
	readonly onRetry: () => void;
	readonly emptyMessage: string;
};

/** Lists the pinned project folders. */
export const ProjectFolders: React.FC<ProjectFoldersProps> = ({
	pinnedProjects,
	organizations,
	chatsByProjectId,
	pinnedChatsByProjectId,
	chatListInput,
	expandedProjectIds,
	onToggle,
	onEdit,
	onDelete,
	isLoading,
	error,
	onRetry,
	emptyMessage,
}) => {
	const location = useLocation();
	const queryClient = useQueryClient();
	const unpinMutation = useMutation({
		...unpinChatProject(queryClient),
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to unpin project."));
		},
	});
	const organizationLabels = getOrganizationLabels(
		pinnedProjects.map((summary) => summary.project),
		organizations,
	);
	return (
		<div className="flex flex-col gap-0.5">
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
			{pinnedProjects.map(({ project, own_unread_count }) => (
				<ProjectFolder
					key={project.id}
					project={project}
					unreadCount={own_unread_count}
					organizationLabel={organizationLabels.get(project.id)}
					chats={chatsByProjectId.get(project.id) ?? []}
					pinnedChats={pinnedChatsByProjectId.get(project.id) ?? []}
					chatListInput={chatListInput}
					expanded={Boolean(expandedProjectIds[project.id])}
					locationSearch={location.search}
					onToggle={() => onToggle(project.id)}
					onEdit={() => onEdit(project)}
					onDelete={() => onDelete(project)}
					onUnpin={() => unpinMutation.mutate(project)}
					emptyMessage={emptyMessage}
				/>
			))}
		</div>
	);
};

type ProjectFolderProps = {
	readonly project: ChatProject;
	readonly unreadCount: number;
	readonly organizationLabel: string | undefined;
	readonly chats: readonly Chat[];
	readonly pinnedChats: readonly Chat[];
	readonly chatListInput: ChatListInput;
	readonly expanded: boolean;
	readonly locationSearch: string;
	readonly onToggle: () => void;
	readonly onEdit: () => void;
	readonly onDelete: () => void;
	readonly onUnpin: () => void;
	readonly emptyMessage: string;
};

const ProjectFolder: React.FC<ProjectFolderProps> = ({
	project,
	unreadCount,
	organizationLabel,
	chats,
	pinnedChats,
	chatListInput,
	expanded,
	locationSearch,
	onToggle,
	onEdit,
	onDelete,
	onUnpin,
	emptyMessage,
}) => {
	const projectPathname = buildAgentProjectPath({ projectId: project.id });
	const projectPath: To = { pathname: projectPathname, search: locationSearch };
	const isOnProjectPage = useMatch(projectPathname) !== null;
	const projectLabel = organizationLabel
		? `${project.name} (${organizationLabel})`
		: project.name;
	const toggleRef = useRef<HTMLButtonElement>(null);
	// Keeps the closing menu from pulling focus back out of the composer.
	const newChatPickedRef = useRef(false);
	const handleNewChat = () => {
		newChatPickedRef.current = true;
	};
	const handleMenuCloseAutoFocus = (event: Event) => {
		if (newChatPickedRef.current) {
			newChatPickedRef.current = false;
			event.preventDefault();
		}
	};
	const menuItemsProps = {
		projectPath,
		onNewChatInPlace: isOnProjectPage ? handleNewChat : undefined,
		onUnpin,
		permissions: project.permissions,
		onEdit,
		onDelete,
	};
	return (
		<ChatDropArea
			target={{
				kind: "project",
				projectId: project.id,
				organizationId: project.organization_id,
				label: projectLabel,
			}}
		>
			<ContextMenu>
				<ContextMenuTrigger asChild>
					<div className="group relative flex items-center gap-1 rounded-md pl-1 pr-2 text-content-secondary group-data-[drop=unavailable]/project-drop:opacity-50 hover:bg-surface-tertiary/50 hover:text-content-primary has-aria-[current=page]:bg-surface-quaternary/50 has-aria-[current=page]:text-content-primary">
						<Button
							ref={toggleRef}
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
							aria-label={
								!expanded && unreadCount > 0
									? `${projectLabel}, ${unreadCount} unread ${unreadCount === 1 ? "chat" : "chats"}`
									: projectLabel
							}
							className="flex min-w-0 flex-1 items-center gap-2 py-1 text-[13px] no-underline"
						>
							{({ isActive }) => (
								<>
									<ChatProjectIcon
										project={project}
										expanded={expanded}
										className="size-4 stroke-[1.75]"
									/>
									<span
										className={cn(
											"flex-1 truncate text-content-primary",
											!isActive &&
												"opacity-85 [@media(hover:hover)]:group-hover:opacity-100 group-data-[state=open]:opacity-100 group-has-data-[state=open]:opacity-100",
										)}
									>
										{project.name}
									</span>
								</>
							)}
						</NavLink>
						{!expanded && unreadCount > 0 && (
							// Covers the actions button, which takes its place on hover
							// or keyboard focus without moving anything.
							<span className="pointer-events-none absolute inset-y-0 right-2 flex w-6 items-center justify-center group-hover:invisible group-has-focus-visible:invisible group-has-data-[state=open]:invisible">
								<span
									aria-hidden="true"
									className="flex h-4 min-w-4 items-center justify-center rounded-full bg-surface-quaternary px-1 text-2xs font-medium text-content-secondary tabular-nums"
								>
									{unreadCount}
								</span>
							</span>
						)}
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									variant="subtle"
									size="icon"
									className="size-6 opacity-0 group-hover:opacity-100 group-has-focus-visible:opacity-100 data-[state=open]:opacity-100"
									aria-label={`Open project actions for ${projectLabel}`}
									{...preventRowContextMenuProps}
								>
									<EllipsisVerticalIcon />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent
								align="end"
								onCloseAutoFocus={handleMenuCloseAutoFocus}
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
									{...menuItemsProps}
								/>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</ContextMenuTrigger>
				<ContextMenuContent onCloseAutoFocus={handleMenuCloseAutoFocus}>
					<ProjectActionsMenuItems
						Item={ContextMenuItem}
						Separator={ContextMenuSeparator}
						{...menuItemsProps}
					/>
				</ContextMenuContent>
			</ContextMenu>
			{expanded && (
				<div className="ml-[15.5px] flex flex-col gap-0.5 border-0 border-l border-solid border-border pl-1.75 pr-2">
					<ProjectFolderChats
						projectId={project.id}
						projectLabel={projectLabel}
						filedChats={chats}
						filedPinnedChats={pinnedChats}
						chatListInput={chatListInput}
						emptyMessage={emptyMessage}
						onRetryFocusLost={() => toggleRef.current?.focus()}
					/>
				</div>
			)}
		</ChatDropArea>
	);
};
