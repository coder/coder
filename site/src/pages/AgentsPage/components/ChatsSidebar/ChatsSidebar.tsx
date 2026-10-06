import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	chatProjects,
	createChatProject,
	deleteChatProject,
	updateChatProject,
} from "#/api/queries/chatProjects";
import { userChatProviderConfigs } from "#/api/queries/chats";
import type { Chat, ChatModel, ChatProject } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import {
	getDefaultOrganizationId,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import type { AgentSidebarFilters } from "../../utils/agentSidebarFilters";
import { draftStorageKeys } from "../AgentCreateForm";
import { AUTOMATIONS_PATH } from "../Automations/automationsFlag";
import { ChatsPanel } from "./chats/ChatsPanel";
import type { ProjectDialogState } from "./chats/ProjectFolders";
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
	onArchiveAgent: (chatId: string) => void;
	onUnarchiveAgent: (chatId: string) => void;
	onArchiveAndDeleteWorkspace: (chatId: string, workspaceId: string) => void;
	onPinAgent: (chatId: string) => void;
	onUnpinAgent: (chatId: string) => void;
	onMarkChatRead: (chatId: string) => void;
	onMarkChatUnread: (chatId: string) => void;
	onReorderPinnedAgent?: (chatId: string, pinOrder: number) => void;
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
	isArchiving?: boolean;
	archivingChatId?: string | null;
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
		onArchiveAgent,
		onUnarchiveAgent,
		onArchiveAndDeleteWorkspace,
		onPinAgent,
		onUnpinAgent,
		onMarkChatRead,
		onMarkChatUnread,
		onReorderPinnedAgent,
		onRenameTitle,
		onProposeTitle,
		chatPendingRename: chatPendingRenameProp,
		onChatPendingRenameChange,
		onBeforeNewAgent,
		isSearchDialogOpen,
		onSearchDialogOpenChange,
		isCreating,
		isArchiving = false,
		archivingChatId = null,
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
	const organizationId: string | undefined =
		getDefaultOrganizationId(organizations) ?? organizations[0]?.id;
	// The sidebar lists the user's projects across organizations and creates
	// new ones in the default organization.
	const chatProjectsEnabled =
		experiments.includes("chat-projects") && organizationId !== undefined;
	const queryClient = useQueryClient();
	const projectsQuery = useQuery({
		...chatProjects(),
		enabled: chatProjectsEnabled,
	});
	const createProjectMutation = useMutation(createChatProject(queryClient));
	const updateProjectMutation = useMutation(updateChatProject(queryClient));
	const deleteProjectMutation = useMutation(deleteChatProject(queryClient));

	// Each dialog keeps its content after closing so the exit animation does
	// not flash empty.
	const [projectDialog, setProjectDialog] = useState<ProjectDialogState>({
		mode: "create",
	});
	const [isProjectDialogOpen, setIsProjectDialogOpen] = useState(false);
	const [deleteDialog, setDeleteDialog] = useState<{
		project: ChatProject | null;
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

	const handleProjectSubmit = (values: ChatProjectFormValues) => {
		if (projectDialog.mode === "edit") {
			updateProjectMutation.mutate(
				{ project: projectDialog.project, request: values },
				{ onSuccess: closeProjectDialog },
			);
			return;
		}
		if (organizationId) {
			createProjectMutation.mutate(
				{ organizationId, request: values },
				{ onSuccess: closeProjectDialog },
			);
		}
	};

	const openDeleteProjectDialog = (project: ChatProject) =>
		setDeleteDialog({ project, open: true });
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

	const { agentId, chatId } = useParams<{
		agentId?: string;
		chatId?: string;
	}>();
	const activeChatId = agentId ?? chatId;
	const location = useLocation();
	const sidebarView = sidebarViewFromPath(location.pathname);
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
				isProjectsLoading={projectsQuery.isLoading}
				projectsError={projectsQuery.error}
				onRetryProjects={() => void projectsQuery.refetch()}
				onOpenProjectDialog={openProjectDialog}
				onDeleteProject={openDeleteProjectDialog}
				chats={chats}
				chatErrorReasons={chatErrorReasons}
				modelConfigs={modelConfigs}
				isLoadingModelConfigs={isLoadingModelConfigs}
				onArchiveAgent={onArchiveAgent}
				onUnarchiveAgent={onUnarchiveAgent}
				onArchiveAndDeleteWorkspace={onArchiveAndDeleteWorkspace}
				onPinAgent={onPinAgent}
				onUnpinAgent={onUnpinAgent}
				onMarkChatRead={onMarkChatRead}
				onMarkChatUnread={onMarkChatUnread}
				onReorderPinnedAgent={onReorderPinnedAgent}
				onBeforeNewAgent={onBeforeNewAgent}
				onOpenSearchDialog={() => onSearchDialogOpenChange(true)}
				onOpenRenameDialog={onRenameTitle ? setChatPendingRename : undefined}
				isCreating={isCreating}
				isArchiving={isArchiving}
				archivingChatId={archivingChatId}
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
				viewedProjectId={
					sidebarView.panel === "chats" ? sidebarView.projectId : undefined
				}
				isSettingsPanel={isSettingsPanel}
				isChatsActive={
					!activeChatId &&
					sidebarView.panel === "chats" &&
					!sidebarView.projectId &&
					!location.pathname.startsWith(AUTOMATIONS_PATH)
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
				confirmLoading={deleteProjectMutation.isPending}
				info="Chats in this project will be kept and move back to the Chats list."
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
