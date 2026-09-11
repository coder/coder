import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useParams } from "react-router";
import {
	chatProjects,
	createChatProject,
	deleteChatProject,
	updateChatProject,
} from "#/api/queries/chatProjects";
import { userChatProviderConfigs } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { Chat, ChatModel } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import type { AgentSidebarFilters } from "../../utils/agentSidebarFilters";
import { ChatsPanel } from "./chats/ChatsPanel";
import { ChatProjectDialog } from "./dialogs/ChatProjectDialog";
import { ChatSearchDialog } from "./dialogs/ChatSearchDialog";
import { RenameChatDialog } from "./dialogs/RenameChatDialog";
import { SettingsPanel } from "./settings/SettingsPanel";
import { isSettingsView, sidebarViewFromPath } from "./sidebarView";

export { isSettingsView, sidebarViewFromPath } from "./sidebarView";

interface ChatsSidebarProps {
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
}

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
	const { organizations, experiments } = useDashboard();
	const defaultOrganizationId =
		organizations.find((organization) => organization.is_default)?.id ?? "";
	const chatProjectsEnabled = experiments.includes("chat-projects");
	const queryClient = useQueryClient();
	const projectsQuery = useQuery({
		...chatProjects(defaultOrganizationId),
		enabled: chatProjectsEnabled && Boolean(defaultOrganizationId),
	});
	const createProjectMutation = useMutation(createChatProject(queryClient));
	const updateProjectMutation = useMutation(updateChatProject(queryClient));
	const deleteProjectMutation = useMutation(deleteChatProject(queryClient));
	const [projectDialogProject, setProjectDialogProject] = useState<
		TypesGen.ChatProject | null | undefined
	>(undefined);
	const [projectPendingDelete, setProjectPendingDelete] =
		useState<TypesGen.ChatProject | null>(null);
	const handleProjectSubmit = async (
		request:
			| TypesGen.CreateChatProjectRequest
			| TypesGen.UpdateChatProjectRequest,
	) => {
		if (projectDialogProject) {
			await updateProjectMutation.mutateAsync({
				projectId: projectDialogProject.id,
				request,
			});
			return;
		}
		if ("organization_id" in request) {
			await createProjectMutation.mutateAsync(request);
		}
	};
	const handleDeleteProject = () => {
		if (!projectPendingDelete) {
			return;
		}
		deleteProjectMutation.mutate(projectPendingDelete.id, {
			onSuccess: () => setProjectPendingDelete(null),
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
				projects={chatProjectsEnabled ? (projectsQuery.data ?? []) : []}
				isProjectsLoading={chatProjectsEnabled && projectsQuery.isLoading}
				onOpenProjectDialog={
					chatProjectsEnabled ? setProjectDialogProject : undefined
				}
				onDeleteProject={
					chatProjectsEnabled ? setProjectPendingDelete : undefined
				}
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
				isSettingsPanel={isSettingsPanel}
				isChatsActive={!activeChatId && sidebarView.panel === "chats"}
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
				organizationId={defaultOrganizationId}
				project={projectDialogProject}
				open={projectDialogProject !== undefined}
				onOpenChange={(open) => {
					if (!open) setProjectDialogProject(undefined);
				}}
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
