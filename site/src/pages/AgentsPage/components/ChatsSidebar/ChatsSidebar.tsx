import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useNavigate, useParams } from "react-router";
import { getErrorMessage } from "#/api/errors";
import {
	chatProjects,
	createChatProject,
	deleteChatProject,
	updateChatProject,
} from "#/api/queries/chatProjects";
import { chatEntityKey, userChatProviderConfigs } from "#/api/queries/chats";
import type { Chat, ChatModel, ChatProject } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { toast } from "#/components/Toaster/toast";
import {
	getDefaultOrganizationId,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import type { AgentSidebarFilters } from "../../utils/agentSidebarFilters";
import { buildAgentProjectPath } from "../../utils/navigation";
import { draftStorageKeys } from "../AgentCreateForm";
import { ChatsPanel } from "./chats/ChatsPanel";
import type { ProjectDialogState } from "./chats/ProjectFolders";
import { getOrganizationLabels } from "./chats/projectGrouping";
import {
	ChatProjectDialog,
	type ChatProjectFormValues,
} from "./dialogs/ChatProjectDialog";
import { ChatSearchDialog } from "./dialogs/ChatSearchDialog";
import { RenameChatDialog } from "./dialogs/RenameChatDialog";
import { SettingsPanel } from "./settings/SettingsPanel";
import { isSettingsView, sidebarViewFromPath } from "./sidebarView";

export { isSettingsView, sidebarViewFromPath } from "./sidebarView";

type ChatsSidebarProps = {
	chats: readonly Chat[];
	chatErrorReasons: Record<string, string>;
	modelConfigs: readonly ChatModel[];
	isLoadingModelConfigs?: boolean;
	onArchiveSuccess?: (chatId: string) => void;
	navigateAfterArchive: (chatId: string) => void;
	onRenameTitle?: (chatId: string, title: string) => Promise<void>;
	onProposeTitle?: (chatId: string) => Promise<string>;
	/**
	 * Controlled value for the rename-chat dialog. When provided alongside
	 * `onChatPendingRenameChange`, the dialog is opened by the parent so
	 * the chat top bar and the sidebar share a single dialog instance.
	 * Falls back to internal state when omitted.
	 */
	chatPendingRename?: Chat | null;
	onChatPendingRenameChange?: (chat: Chat | null) => void;
	onBeforeNewAgent?: () => void;
	isSearchDialogOpen: boolean;
	onSearchDialogOpenChange: (open: boolean) => void;
	isCreating: boolean;
	isLoading?: boolean;
	loadError?: unknown;
	onRetryLoad?: () => void;
	hasNextPage?: boolean;
	onLoadMore?: () => void;
	isFetchingNextPage?: boolean;
	sidebarFilters: AgentSidebarFilters;
	onSidebarFiltersChange: (filters: AgentSidebarFilters) => void;
	onCollapse?: () => void;
	isPersonalModelOverridesEnabled?: boolean;
	isAdmin?: boolean;
	/**
	 * Whether the user can open the Coder Agents settings page. Broader
	 * than isAdmin: organization model admins qualify without deployment
	 * config access.
	 */
	canManageAgentSettings?: boolean;
	currentUserId: string;
};

export const ChatsSidebar: React.FC<ChatsSidebarProps> = (props) => {
	const {
		chats,
		chatErrorReasons,
		modelConfigs,
		isLoadingModelConfigs = false,
		onArchiveSuccess,
		navigateAfterArchive,
		onRenameTitle,
		onProposeTitle,
		chatPendingRename: chatPendingRenameProp,
		onChatPendingRenameChange,
		onBeforeNewAgent,
		isSearchDialogOpen,
		onSearchDialogOpenChange,
		isCreating,
		isLoading = false,
		loadError,
		onRetryLoad,
		hasNextPage,
		onLoadMore,
		isFetchingNextPage,
		sidebarFilters,
		onSidebarFiltersChange,
		onCollapse,
		isPersonalModelOverridesEnabled = false,
		isAdmin = false,
		canManageAgentSettings = false,
		currentUserId,
	} = props;
	const { organizations, experiments } = useDashboard();
	const initialOrganizationId =
		getDefaultOrganizationId(organizations) ?? organizations[0]?.id;
	// The sidebar lists the user's projects across organizations.
	const chatProjectsEnabled =
		experiments.includes("chat-projects") &&
		initialOrganizationId !== undefined;
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const location = useLocation();
	const sidebarView = sidebarViewFromPath(location.pathname);
	const viewedProjectId =
		sidebarView.panel === "chats" ? sidebarView.projectId : undefined;
	const projectsQuery = useQuery({
		...chatProjects(),
		enabled: chatProjectsEnabled,
	});
	const createProjectMutation = useMutation(createChatProject(queryClient));
	const updateProjectMutation = useMutation(updateChatProject(queryClient));
	const { agentId, chatId } = useParams<{
		agentId?: string;
		chatId?: string;
	}>();
	const activeChatId = agentId ?? chatId;
	const deleteProjectOptions = deleteChatProject(queryClient);
	const deleteProjectMutation = useMutation({
		...deleteProjectOptions,
		// Leaves the deleted project's page, or one of its chats, before the
		// caches refetch, so the page never shows "not found". A pending
		// mutation picks up each render's options, so onSuccess reads the
		// current location.
		onMutate: (project) => {
			// Read before the request, since a project_deleted event can
			// reset the entity before the response arrives.
			const openChat = activeChatId
				? queryClient.getQueryData<Chat>(chatEntityKey(activeChatId))
				: undefined;
			return {
				openChatId:
					openChat?.project_id === project.id ? openChat.id : undefined,
			};
		},
		onSuccess: (data, project, context) => {
			deleteProjectOptions.onSuccess?.(data, project, context);
			if (
				viewedProjectId === project.id ||
				(context?.openChatId !== undefined &&
					context.openChatId === activeChatId)
			) {
				navigate(
					{ pathname: "/agents", search: location.search },
					{ replace: true },
				);
			}
		},
	});

	// Each dialog keeps its content after closing so the exit animation does
	// not flash empty.
	const [projectDialog, setProjectDialog] = useState<ProjectDialogState>({
		mode: "create",
	});
	const [isProjectDialogOpen, setIsProjectDialogOpen] = useState(false);
	const [deleteDialog, setDeleteDialog] = useState<{
		project: ChatProject | null;
		organizationLabel?: string;
		open: boolean;
	}>({ project: null, open: false });

	const openProjectDialog = (dialog: ProjectDialogState) => {
		if (dialog.mode === "create") {
			createProjectMutation.reset();
		} else {
			updateProjectMutation.reset();
		}
		setProjectDialog(dialog);
		setIsProjectDialogOpen(true);
	};
	const closeProjectDialog = () => setIsProjectDialogOpen(false);

	const handleProjectSubmit = ({
		organizationId,
		...request
	}: ChatProjectFormValues) => {
		if (projectDialog.mode === "edit") {
			updateProjectMutation.mutate(
				{ project: projectDialog.project, request },
				{ onSuccess: closeProjectDialog },
			);
			return;
		}
		if (organizationId) {
			createProjectMutation.mutate(
				{ organizationId, request },
				{
					onSuccess: (project) => {
						closeProjectDialog();
						navigate({
							pathname: buildAgentProjectPath({ projectId: project.id }),
							search: location.search,
						});
					},
				},
			);
		}
	};

	// The label is captured on open because deleting the project can end the
	// name collision before the dialog finishes closing.
	const openDeleteProjectDialog = (project: ChatProject) =>
		setDeleteDialog({
			project,
			organizationLabel: getOrganizationLabels(
				projectsQuery.data ?? [],
				organizations,
			).get(project.id),
			open: true,
		});
	const closeDeleteProjectDialog = () =>
		setDeleteDialog((current) => ({ ...current, open: false }));

	const handleDeleteProject = () => {
		const project = deleteDialog.project;
		if (!project) {
			return;
		}
		deleteProjectMutation.mutate(project, {
			onSuccess: () => {
				// Nothing can open the deleted project's composer again.
				const draftKeys = draftStorageKeys(project.id);
				localStorage.removeItem(draftKeys.text);
				localStorage.removeItem(draftKeys.attachments);
				// The dialog can be dismissed while the delete is in flight and
				// reopened for another project, which must stay open.
				setDeleteDialog((current) =>
					current.project?.id === project.id
						? { ...current, open: false }
						: current,
				);
			},
			onError: (error) => {
				toast.error(getErrorMessage(error, "Failed to delete project."));
			},
		});
	};

	const isSettingsPanel = isSettingsView(sidebarView);
	const settingsSection = isSettingsPanel ? sidebarView.section : undefined;
	const providerConfigsQuery = useQuery({
		...userChatProviderConfigs(),
		enabled: isSettingsPanel && !isAdmin,
	});
	const isApiKeysSection = isSettingsPanel && settingsSection === "api-keys";
	const showApiKeysItem =
		isAdmin || isApiKeysSection || Boolean(providerConfigsQuery.data?.length);
	const [internalChatPendingRename, setInternalChatPendingRename] =
		useState<Chat | null>(null);
	const isControlled = chatPendingRenameProp !== undefined;
	const chatPendingRename = isControlled
		? chatPendingRenameProp
		: internalChatPendingRename;
	const setChatPendingRename = (chat: Chat | null) => {
		if (isControlled) {
			onChatPendingRenameChange?.(chat);
		} else {
			setInternalChatPendingRename(chat);
		}
	};

	return (
		<div className="relative flex size-full min-h-0 border-0 border-r border-solid overflow-hidden">
			<ChatsPanel
				chatProjectsEnabled={chatProjectsEnabled}
				projects={projectsQuery.data ?? []}
				organizations={organizations}
				isProjectsLoading={projectsQuery.isLoading}
				projectsError={projectsQuery.error}
				isProjectsLoadError={projectsQuery.isLoadingError}
				onRetryProjects={() => void projectsQuery.refetch()}
				onOpenProjectDialog={openProjectDialog}
				onDeleteProject={openDeleteProjectDialog}
				chats={chats}
				chatErrorReasons={chatErrorReasons}
				modelConfigs={modelConfigs}
				isLoadingModelConfigs={isLoadingModelConfigs}
				onArchiveSuccess={onArchiveSuccess}
				navigateAfterArchive={navigateAfterArchive}
				onBeforeNewAgent={onBeforeNewAgent}
				onOpenSearchDialog={() => onSearchDialogOpenChange(true)}
				onOpenRenameDialog={onRenameTitle ? setChatPendingRename : undefined}
				isCreating={isCreating}
				isLoading={isLoading}
				loadError={loadError}
				onRetryLoad={onRetryLoad}
				hasNextPage={hasNextPage}
				onLoadMore={onLoadMore}
				isFetchingNextPage={isFetchingNextPage}
				sidebarFilters={sidebarFilters}
				onSidebarFiltersChange={onSidebarFiltersChange}
				onCollapse={onCollapse}
				activeChatId={activeChatId}
				viewedProjectId={viewedProjectId}
				isSettingsPanel={isSettingsPanel}
				isChatsActive={
					!activeChatId &&
					sidebarView.panel === "chats" &&
					!sidebarView.projectId
				}
				location={location}
				currentUserId={currentUserId}
			/>
			<SettingsPanel
				isSettingsPanel={isSettingsPanel}
				settingsSection={settingsSection}
				showApiKeysItem={showApiKeysItem}
				isPersonalModelOverridesEnabled={isPersonalModelOverridesEnabled}
				canManageAgentSettings={canManageAgentSettings}
				location={location}
				onCollapse={onCollapse}
			/>
			<ChatSearchDialog
				open={isSearchDialogOpen}
				onOpenChange={onSearchDialogOpenChange}
				location={location}
				recentChats={chats}
			/>
			<ChatProjectDialog
				organizations={organizations}
				initialOrganizationId={initialOrganizationId}
				project={
					projectDialog.mode === "edit" ? projectDialog.project : undefined
				}
				open={isProjectDialogOpen}
				onOpenChange={(open) => {
					if (!open) closeProjectDialog();
				}}
				isSubmitting={
					projectDialog.mode === "edit"
						? updateProjectMutation.isPending
						: createProjectMutation.isPending
				}
				error={
					projectDialog.mode === "edit"
						? updateProjectMutation.error
						: createProjectMutation.error
				}
				onSubmit={handleProjectSubmit}
			/>
			<DeleteDialog
				isOpen={deleteDialog.open}
				onConfirm={handleDeleteProject}
				onCancel={closeDeleteProjectDialog}
				entity="project"
				name={deleteDialog.project?.name ?? ""}
				title={
					deleteDialog.organizationLabel
						? `Delete "${deleteDialog.project?.name}" from ${deleteDialog.organizationLabel}`
						: undefined
				}
				confirmLoading={deleteProjectMutation.isPending}
				info="Its memories, instructions and skills are deleted now. Every chat in it, including chats started by people it is shared with, is stopped and can't be opened again. If conversation retention is enabled, the chats are permanently deleted when the retention period passes."
			/>
			{onRenameTitle && (
				<RenameChatDialog
					chat={chatPendingRename}
					onRename={onRenameTitle}
					onPropose={onProposeTitle}
					onOpenChange={(open: boolean) => {
						if (!open) setChatPendingRename(null);
					}}
				/>
			)}
		</div>
	);
};
