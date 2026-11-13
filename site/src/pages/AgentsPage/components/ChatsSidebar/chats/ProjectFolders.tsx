import { useDragDropMonitor } from "@dnd-kit/react";
import { isSortable, useSortable } from "@dnd-kit/react/sortable";
import { cn } from "cn";
import { ChevronRightIcon, EllipsisVerticalIcon } from "lucide-react";
import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { NavLink, type To, useLocation, useMatch } from "react-router";
import { getErrorMessage } from "#/api/errors";
import { pinChatProject, unpinChatProject } from "#/api/queries/chatProjects";
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
import {
	applyLocalPinOrder,
	movePinnedId,
	PROJECT_PIN_DRAG_TYPE,
	type ProjectPinDragData,
	pinnedProjectOf,
} from "./projectDrag";
import { getOrganizationLabels } from "./projectGrouping";

/** Acts on one project, such as editing or deleting it. */
export type ProjectAction = (project: ChatProject) => void;
/**
 * `trigger` is the folder's actions button, to refocus if it remains.
 * Archiving removes the folder, so focusAfterRemoval then moves focus on.
 */
export type ArchiveProjectAction = (
	project: ChatProject,
	trigger: HTMLButtonElement | null,
	focusAfterRemoval: () => void,
) => void;

export type ProjectFoldersProps = {
	/** The user's pinned projects, in pin order. */
	readonly pinnedProjects: readonly ChatProjectSummary[];
	/** Describes how to reorder a pinned project with the keyboard. */
	readonly dragInstructionsId: string;
	readonly organizations: readonly Organization[];
	readonly chatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	/** The Pinned section lists these, so folders only count them. */
	readonly pinnedChatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	readonly chatListInput: ChatListInput;
	readonly expandedProjectIds: Readonly<Record<string, boolean>>;
	readonly onToggle: (projectId: string) => void;
	readonly onEdit: ProjectAction;
	readonly onArchive: ArchiveProjectAction;
	readonly onDelete: ProjectAction;
	readonly isLoading: boolean;
	readonly error: unknown;
	readonly onRetry: () => void;
	readonly emptyMessage: string;
	/**
	 * Moves focus out of the folders when unpinning or archiving removes the
	 * last one.
	 */
	readonly focusUnpinFallback?: () => void;
};

// Focuses the folder after a removed one, else the one before it, else the
// fallback.
const focusNextFolder = (
	pinnedProjects: readonly ChatProjectSummary[],
	projectId: string,
	toggles: ReadonlyMap<string, HTMLButtonElement>,
	fallback: (() => void) | undefined,
) => {
	const ids = pinnedProjects.map((summary) => summary.project.id);
	const index = ids.indexOf(projectId);
	const neighborId = ids[index + 1] ?? ids[index - 1];
	const neighbor = neighborId ? toggles.get(neighborId) : undefined;
	if (neighbor) {
		neighbor.focus();
	} else {
		fallback?.();
	}
};

/** Lists the pinned project folders, which drag to reorder the pins. */
export const ProjectFolders: React.FC<ProjectFoldersProps> = ({
	pinnedProjects,
	dragInstructionsId,
	organizations,
	chatsByProjectId,
	pinnedChatsByProjectId,
	chatListInput,
	expandedProjectIds,
	onToggle,
	onEdit,
	onArchive,
	onDelete,
	isLoading,
	error,
	onRetry,
	emptyMessage,
	focusUnpinFallback,
}) => {
	const location = useLocation();
	const queryClient = useQueryClient();
	const pinOptions = pinChatProject(queryClient);
	const reorderMutation = useMutation({
		...pinOptions,
		onError: (error, variables, context) => {
			pinOptions.onError?.(error, variables, context);
			toast.error(getErrorMessage(error, "Failed to reorder pinned projects."));
		},
	});
	const unpinOptions = unpinChatProject(queryClient);
	const unpinMutation = useMutation({
		...unpinOptions,
		onError: (error, project, context) => {
			unpinOptions.onError?.(error, project, context);
			toast.error(getErrorMessage(error, "Failed to unpin project."));
		},
	});
	// Holds a drop's order until the reordered summaries reach the cache, so
	// the folders do not jump back while the drop renders.
	const [localPinOrder, setLocalPinOrder] = useState<string[] | null>(null);
	const [seenPinnedProjects, setSeenPinnedProjects] = useState(pinnedProjects);
	if (pinnedProjects !== seenPinnedProjects) {
		setSeenPinnedProjects(pinnedProjects);
		setLocalPinOrder(null);
	}
	const sortedPinnedProjects = applyLocalPinOrder(
		pinnedProjects,
		localPinOrder,
		(summary) => summary.project.id,
	);
	useDragDropMonitor({
		onDragEnd: ({ operation: { source }, canceled }) => {
			const project = pinnedProjectOf(source)?.project;
			if (canceled || !project || !isSortable(source)) {
				return;
			}
			const order = movePinnedId(
				sortedPinnedProjects.map((summary) => summary.project.id),
				project.id,
				source.index,
			);
			if (order) {
				setLocalPinOrder(order);
				reorderMutation.mutate({ project, pinOrder: source.index + 1 });
			}
		},
	});
	const toggleRefs = useRef(new Map<string, HTMLButtonElement>());
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
			{sortedPinnedProjects.map(({ project, own_unread_count }, index) => (
				<SortableProjectFolder
					key={project.id}
					project={project}
					index={index}
					dragInstructionsId={dragInstructionsId}
					unreadCount={own_unread_count}
					organizationLabel={organizationLabels.get(project.id)}
					chats={chatsByProjectId.get(project.id) ?? []}
					pinnedChats={pinnedChatsByProjectId.get(project.id) ?? []}
					chatListInput={chatListInput}
					expanded={Boolean(expandedProjectIds[project.id])}
					locationSearch={location.search}
					onToggle={() => onToggle(project.id)}
					onEdit={() => onEdit(project)}
					onArchive={(trigger) =>
						onArchive(project, trigger, () =>
							focusNextFolder(
								sortedPinnedProjects,
								project.id,
								toggleRefs.current,
								focusUnpinFallback,
							),
						)
					}
					onDelete={() => onDelete(project)}
					onUnpin={() => unpinMutation.mutate(project)}
					focusAfterUnpin={() =>
						focusNextFolder(
							sortedPinnedProjects,
							project.id,
							toggleRefs.current,
							focusUnpinFallback,
						)
					}
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
	);
};

type ProjectFolderProps = {
	readonly project: ChatProject;
	/** Makes the project link the drag handle that reorders the pins. */
	readonly handleRef: (element: Element | null) => void;
	readonly dragInstructionsId: string;
	readonly unreadCount: number;
	readonly organizationLabel: string | undefined;
	readonly chats: readonly Chat[];
	readonly pinnedChats: readonly Chat[];
	readonly chatListInput: ChatListInput;
	readonly expanded: boolean;
	readonly locationSearch: string;
	readonly onToggle: () => void;
	readonly onEdit: () => void;
	readonly onArchive: (trigger: HTMLButtonElement | null) => void;
	readonly onDelete: () => void;
	readonly onUnpin: () => void;
	readonly emptyMessage: string;
	/** Moves focus out of the folder, which unpinning removes. */
	readonly focusAfterUnpin: () => void;
	readonly registerToggle: (element: HTMLButtonElement | null) => void;
};

/** Names a project, adding its organization when another project shares the name. */
export const projectLabelOf = (
	project: ChatProject,
	organizationLabel?: string,
) =>
	organizationLabel ? `${project.name} (${organizationLabel})` : project.name;

type SortableProjectFolderProps = Omit<ProjectFolderProps, "handleRef"> & {
	readonly index: number;
};

const SortableProjectFolder: React.FC<SortableProjectFolderProps> = ({
	index,
	...props
}) => {
	const { ref, handleRef, isDragging } = useSortable({
		id: `project-pin:${props.project.id}`,
		index,
		// Its own group, so pinned projects never sort among pinned chats.
		group: PROJECT_PIN_DRAG_TYPE,
		type: PROJECT_PIN_DRAG_TYPE,
		accept: PROJECT_PIN_DRAG_TYPE,
		data: {
			project: props.project,
			label: projectLabelOf(props.project, props.organizationLabel),
		} satisfies ProjectPinDragData,
	});
	return (
		<div ref={ref} className={cn(isDragging && "opacity-50")}>
			<ProjectFolder {...props} handleRef={handleRef} />
		</div>
	);
};

const ProjectFolder: React.FC<ProjectFolderProps> = ({
	project,
	handleRef,
	dragInstructionsId,
	unreadCount,
	organizationLabel,
	chats,
	pinnedChats,
	chatListInput,
	expanded,
	locationSearch,
	onToggle,
	onEdit,
	onArchive,
	onDelete,
	onUnpin,
	emptyMessage,
	focusAfterUnpin,
	registerToggle,
}) => {
	const projectPathname = buildAgentProjectPath({ projectId: project.id });
	const projectPath: To = { pathname: projectPathname, search: locationSearch };
	const isOnProjectPage = useMatch(projectPathname) !== null;
	const projectLabel = projectLabelOf(project, organizationLabel);
	const toggleRef = useRef<HTMLButtonElement>(null);
	const actionsButtonRef = useRef<HTMLButtonElement>(null);
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
	// The archive dialog opens only once the menu has closed, so the closing
	// menu cannot take focus back from it.
	const archivePickedRef = useRef(false);
	const handleMenuCloseAutoFocus = (event: Event) => {
		if (archivePickedRef.current) {
			archivePickedRef.current = false;
			event.preventDefault();
			onArchive(actionsButtonRef.current);
		}
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
		project,
		onEdit,
		onArchive: () => {
			archivePickedRef.current = true;
		},
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
							ref={handleRef}
							to={projectPath}
							aria-label={
								!expanded && unreadCount > 0
									? `${projectLabel}, ${unreadCount} unread ${unreadCount === 1 ? "chat" : "chats"}`
									: projectLabel
							}
							aria-describedby={dragInstructionsId}
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
							<span
								aria-hidden="true"
								className={cn(
									"pointer-events-none absolute inset-y-0 right-2 flex min-w-6 items-center justify-end gap-1.5",
									"text-[13px] text-content-secondary tabular-nums",
									"group-hover:invisible group-has-focus-visible:invisible group-has-data-[state=open]:invisible",
								)}
							>
								<span className="size-1.5 rounded-full bg-content-link" />
								{unreadCount}
							</span>
						)}
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									ref={actionsButtonRef}
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
				<div className="ml-[15.5px] flex flex-col gap-0.5 border-l pl-1.75 pr-2">
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
