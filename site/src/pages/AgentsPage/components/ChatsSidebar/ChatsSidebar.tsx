import { type FC, useRef, useState } from "react";
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
import { permittedOrganizations } from "#/api/queries/organizations";
import type * as TypesGen from "#/api/typesGenerated";
import type { Chat, ChatModel } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import {
	getDefaultOrganizationId,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import type { AgentSidebarFilters } from "../../utils/agentSidebarFilters";
import { ChatsPanel } from "./chats/ChatsPanel";
import type { ProjectDialogMode } from "./chats/ProjectFolders";
import { ChatProjectDialog } from "./dialogs/ChatProjectDialog";
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

export const ChatsSidebar: FC<ChatsSidebarProps> = (props) => {
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
	const { organizations, showOrganizations, experiments } = useDashboard();
	const organizationId: string | undefined =
		getDefaultOrganizationId(organizations) ?? organizations[0]?.id;
	const chatProjectsEnabled = experiments.includes("chat-projects");
	const queryClient = useQueryClient();
	// Same organizations the composer offers for a new chat. The selector
	// appears only when this list has more than one entry.
	const permittedOrgsQuery = useQuery({
		...permittedOrganizations({
			object: { resource_type: "chat", owner_id: "me" },
			action: "create",
		}),
		enabled: chatProjectsEnabled && showOrganizations,
	});
	const creatableOrganizations = showOrganizations
		? (permittedOrgsQuery.data ?? [])
		: organizations;
	// The sidebar lists projects in the default organization. Create sends
	// the organization chosen in the dialog.
	const projectsQuery = useQuery({
		...chatProjects(organizationId),
		enabled: chatProjectsEnabled && organizationId !== undefined,
	});
	const createProjectMutation = useMutation(createChatProject(queryClient));
	const updateProjectMutation = useMutation(updateChatProject(queryClient));
	const deleteProjectMutation = useMutation(deleteChatProject(queryClient));
	const [projectDialog, setProjectDialog] = useState<ProjectDialogMode | null>(
		null,
	);
	const projectDialogTriggerRef = useRef<HTMLElement | null>(null);
	const [projectPendingDelete, setProjectPendingDelete] =
		useState<TypesGen.ChatProject | null>(null);
	const closeProjectDialog = () => {
		setProjectDialog(null);
		requestAnimationFrame(() => projectDialogTriggerRef.current?.focus());
	};
	const openProjectDialog = (dialog: ProjectDialogMode) => {
		projectDialogTriggerRef.current =
			document.activeElement instanceof HTMLElement
				? document.activeElement
				: null;
		if (dialog.mode === "create") {
			const selectionSettled =
				!showOrganizations ||
				permittedOrgsQuery.isFetched ||
				permittedOrgsQuery.isError;
			if (selectionSettled && creatableOrganizations.length === 0) {
				return;
			}
			createProjectMutation.reset();
		} else {
			updateProjectMutation.reset();
		}
		setProjectDialog(dialog);
	};
	const handleProjectSubmit = (request: {
		name: string;
		description: string;
		icon: string;
		organization_id?: string;
	}) => {
		if (projectDialog?.mode === "edit") {
			updateProjectMutation.mutate(
				{
					projectId: projectDialog.project.id,
					request: {
						name: request.name,
						description: request.description,
						icon: request.icon,
					},
				},
				{ onSuccess: closeProjectDialog },
			);
			return;
		}
		if (projectDialog?.mode === "create" && request.organization_id) {
			createProjectMutation.mutate(
				{
					organization_id: request.organization_id,
					name: request.name,
					description: request.description,
					icon: request.icon,
				},
				{ onSuccess: closeProjectDialog },
			);
		}
	};
	const handleDeleteProject = () => {
		if (!projectPendingDelete) {
			return;
		}
		const deletedId = projectPendingDelete.id;
		deleteProjectMutation.mutate(deletedId, {
			onSuccess: () =>
				setProjectPendingDelete((current) =>
					current?.id === deletedId ? null : current,
				),
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
				chatProjectsEnabled={
					chatProjectsEnabled && organizationId !== undefined
				}
				projects={
					chatProjectsEnabled
						? (projectsQuery.data ?? []).filter(
								(project) => project.owner_id === currentUserId,
							)
						: []
				}
				isProjectsLoading={chatProjectsEnabled && projectsQuery.isLoading}
				projectsError={chatProjectsEnabled ? projectsQuery.error : undefined}
				onRetryProjects={() => void projectsQuery.refetch()}
				onOpenProjectDialog={openProjectDialog}
				onDeleteProject={setProjectPendingDelete}
				chats={chats}
				chatErrorReasons={chatErrorReasons}
				modelConfigs={modelConfigs}
				isLoadingModelConfigs={isLoadingModelConfigs}
				onArchiveAgent={onArchiveAgent}
				onUnarchiveAgent={onUnarchiveAgent}
				onArchiveAndDeleteWorkspace={onArchiveAndDeleteWorkspace}
				onPinAgent={onPinAgent}
				onUnpinAgent={onUnpinAgent}
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
				project={
					projectDialog?.mode === "edit" ? projectDialog.project : undefined
				}
				organizations={
					projectDialog?.mode === "create" ? creatableOrganizations : []
				}
				open={projectDialog !== null}
				onOpenChange={(open) => {
					if (!open) closeProjectDialog();
				}}
				isSubmitting={
					projectDialog?.mode === "edit"
						? updateProjectMutation.isPending
						: createProjectMutation.isPending
				}
				error={
					projectDialog?.mode === "edit"
						? updateProjectMutation.error
						: createProjectMutation.error
				}
				onSubmit={handleProjectSubmit}
			/>
			<DeleteDialog
				isOpen={projectPendingDelete !== null}
				onConfirm={handleDeleteProject}
				onCancel={() => setProjectPendingDelete(null)}
				entity="project"
				name={projectPendingDelete?.name ?? ""}
				confirmLoading={deleteProjectMutation.isPending}
				info="Chats in this project will be kept and become independent."
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
