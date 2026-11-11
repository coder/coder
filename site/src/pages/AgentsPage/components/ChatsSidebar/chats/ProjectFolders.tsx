import { cn } from "cn";
import { ChevronRightIcon, EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import { type RefObject, useRef, useState } from "react";
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
import {
	getPersistedProjectsSectionExpanded,
	savePersistedProjectsSectionExpanded,
} from "../../../utils/projectsSectionStorage";
import { ChatProjectIcon } from "../../ChatProjectIcon";
import { ChatDropArea } from "./ChatDropArea";
import { ProjectActionsMenuItems } from "./ProjectActionsMenuItems";
import { ProjectFolderChats } from "./ProjectFolderChats";
import { ProjectsSectionDropArea } from "./ProjectsSectionDropArea";
import { getOrganizationLabels } from "./projectGrouping";
import { SidebarSectionHeader } from "./SidebarSectionHeader";

export type ProjectDialogState =
	| { mode: "create" }
	| { mode: "edit"; project: ChatProject };

export type OpenProjectDialog = (dialog: ProjectDialogState) => void;
export type DeleteProject = (project: ChatProject) => void;

type ProjectFoldersProps = {
	readonly currentUserId: string;
	/** The user's pinned projects, in pin order. */
	readonly pinnedProjects: readonly ChatProjectSummary[];
	readonly organizations: readonly Organization[];
	readonly chatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	/** The Pinned section lists these, so folders only count them. */
	readonly pinnedChatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	readonly chatListInput: ChatListInput;
	readonly expandedProjectIds: Readonly<Record<string, boolean>>;
	readonly onToggle: (projectId: string) => void;
	readonly onOpenProjectDialog: OpenProjectDialog;
	readonly onDelete: DeleteProject;
	readonly isLoading: boolean;
	readonly error: unknown;
	/**
	 * True when the first load failed. A failed refetch keeps cached
	 * projects, including an empty list, so its empty state stays.
	 */
	readonly isLoadError: boolean;
	readonly onRetry: () => void;
	readonly emptyMessage: string;
	/** Takes focus when the user unpins the last pinned project. */
	readonly unpinFocusFallbackRef?: RefObject<HTMLElement | null>;
};

export const ProjectFolders: React.FC<ProjectFoldersProps> = ({
	currentUserId,
	pinnedProjects,
	organizations,
	chatsByProjectId,
	pinnedChatsByProjectId,
	chatListInput,
	expandedProjectIds,
	onToggle,
	onOpenProjectDialog,
	onDelete,
	isLoading,
	error,
	isLoadError,
	onRetry,
	emptyMessage,
	unpinFocusFallbackRef,
}) => {
	const location = useLocation();
	const queryClient = useQueryClient();
	const unpinMutation = useMutation({
		...unpinChatProject(queryClient),
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to unpin project."));
		},
	});
	const toggleRefs = useRef(new Map<string, HTMLButtonElement>());
	const focusAfterUnpin = (projectId: string) => {
		const ids = pinnedProjects.map((summary) => summary.project.id);
		const index = ids.indexOf(projectId);
		const neighborId = ids[index + 1] ?? ids[index - 1];
		const neighbor = neighborId
			? toggleRefs.current.get(neighborId)
			: undefined;
		(neighbor ?? unpinFocusFallbackRef?.current)?.focus();
	};
	const organizationLabels = getOrganizationLabels(
		pinnedProjects.map((summary) => summary.project),
		organizations,
	);
	const [isExpanded, setIsExpanded] = useState(() =>
		getPersistedProjectsSectionExpanded(currentUserId),
	);
	const setExpanded = (expanded: boolean) => {
		setIsExpanded(expanded);
		savePersistedProjectsSectionExpanded(currentUserId, expanded);
	};
	const openCreateDialog = () => onOpenProjectDialog({ mode: "create" });
	const isShowingEmptyState =
		isExpanded && !isLoading && !isLoadError && pinnedProjects.length === 0;
	// Also hidden while an expanded section loads: if the load comes back
	// empty, the empty state's button would replace a focused header button
	// and drop keyboard focus.
	const isShowingHeaderNewProject =
		!isShowingEmptyState && !(isExpanded && isLoading);

	return (
		<div>
			<ProjectsSectionDropArea
				isExpanded={isExpanded}
				onExpand={() => setExpanded(true)}
			>
				<SidebarSectionHeader
					title="Projects"
					collapsible={{
						expanded: isExpanded,
						onToggle: () => setExpanded(!isExpanded),
					}}
					actions={
						isShowingHeaderNewProject && (
							<Button
								variant="subtle"
								size="icon"
								className="size-7"
								aria-label="New project"
								onClick={openCreateDialog}
							>
								<PlusIcon />
							</Button>
						)
					}
				/>
			</ProjectsSectionDropArea>
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
					{isShowingEmptyState && (
						<div className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border mx-1 px-8 py-5 text-center">
							<p className="m-0 text-sm text-content-secondary">
								Group related chats together, with shared members and history.
							</p>
							<Button variant="outline" size="sm" onClick={openCreateDialog}>
								<PlusIcon />
								New project
							</Button>
						</div>
					)}
					{pinnedProjects.length > 0 && (
						<div className="flex flex-col gap-0.5">
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
									onEdit={() => onOpenProjectDialog({ mode: "edit", project })}
									onDelete={() => onDelete(project)}
									onUnpin={() => unpinMutation.mutate(project)}
									focusAfterUnpin={() => focusAfterUnpin(project.id)}
									registerToggle={(element) => {
										if (element) {
											toggleRefs.current.set(project.id, element);
										} else {
											toggleRefs.current.delete(project.id);
										}
									}}
									emptyMessage={emptyMessage}
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
	/** Moves focus out of the folder, which unpinning removes. */
	readonly focusAfterUnpin: () => void;
	readonly registerToggle: (element: HTMLButtonElement | null) => void;
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
	focusAfterUnpin,
	registerToggle,
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
	// Unpinning removes this folder, so the closing menu cannot return focus
	// to its button.
	const unpinPickedRef = useRef(false);
	const handleUnpin = () => {
		unpinPickedRef.current = true;
		onUnpin();
	};
	const handleMenuCloseAutoFocus = (event: Event) => {
		if (newChatPickedRef.current) {
			newChatPickedRef.current = false;
			event.preventDefault();
		}
		if (unpinPickedRef.current) {
			unpinPickedRef.current = false;
			event.preventDefault();
			focusAfterUnpin();
		}
	};
	const menuItemsProps = {
		projectPath,
		onNewChatInPlace: isOnProjectPage ? handleNewChat : undefined,
		onUnpin: handleUnpin,
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
					<div className="group relative flex items-center gap-1 rounded-md pl-1 pr-2 text-content-secondary group-data-[drop=unavailable]/drop:opacity-50 hover:bg-surface-tertiary/50 hover:text-content-primary has-aria-[current=page]:bg-surface-quaternary/50 has-aria-[current=page]:text-content-primary">
						<Button
							ref={(element) => {
								toggleRef.current = element;
								registerToggle(element);
							}}
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
