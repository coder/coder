import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { useLocation, useParams } from "react-router";
import { userChatProviderConfigs } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { ChatsPanel } from "./chats/ChatsPanel";
import { RenameChatDialog } from "./dialogs/RenameChatDialog";
import { SettingsPanel } from "./settings/SettingsPanel";
import { isSettingsView, sidebarViewFromPath } from "./sidebarView";
import type { ChatsSidebarProps } from "./types";

export { isSettingsView, sidebarViewFromPath } from "./sidebarView";

export const ChatsSidebar: FC<ChatsSidebarProps> = (props) => {
	const {
		chats,
		chatErrorReasons,
		modelOptions,
		modelConfigs,
		onArchiveAgent,
		onUnarchiveAgent,
		onArchiveAndDeleteWorkspace,
		onPinAgent,
		onUnpinAgent,
		onReorderPinnedAgent,
		onRenameTitle,
		onProposeTitle,
		onBeforeNewAgent,
		isCreating,
		isArchiving = false,
		archivingChatId = null,
		regeneratingTitleChatIds,
		isLoading = false,
		loadError,
		onRetryLoad,
		hasNextPage,
		onLoadMore,
		isFetchingNextPage,
		archivedFilter,
		onArchivedFilterChange,
		onCollapse,
		isPersonalModelOverridesEnabled = false,
		isAdmin = false,
	} = props;
	const { agentId, chatId } = useParams<{
		agentId?: string;
		chatId?: string;
	}>();
	const activeChatId = agentId ?? chatId;
	const location = useLocation();
	const sidebarView = sidebarViewFromPath(location.pathname);
	const isSettingsPanel = isSettingsView(sidebarView);
	const isFallbackToUserPanel =
		sidebarView.panel === "settings-admin" && !isAdmin;
	const settingsPanel =
		sidebarView.panel === "settings-admin" && isAdmin
			? "settings-admin"
			: "settings";
	const settingsSection =
		isSettingsPanel && !isFallbackToUserPanel ? sidebarView.section : undefined;
	const providerConfigsQuery = useQuery({
		...userChatProviderConfigs(),
		enabled: isSettingsPanel && !isAdmin,
	});
	const isApiKeysSection = isSettingsPanel && settingsSection === "api-keys";
	const showApiKeysItem =
		isAdmin || isApiKeysSection || Boolean(providerConfigsQuery.data?.length);
	const [chatPendingRename, setChatPendingRename] = useState<Chat | null>(null);

	return (
		<div className="relative flex h-full w-full min-h-0 border-0 border-r border-solid overflow-hidden">
			<ChatsPanel
				chats={chats}
				chatErrorReasons={chatErrorReasons}
				modelOptions={modelOptions}
				modelConfigs={modelConfigs}
				onArchiveAgent={onArchiveAgent}
				onUnarchiveAgent={onUnarchiveAgent}
				onArchiveAndDeleteWorkspace={onArchiveAndDeleteWorkspace}
				onPinAgent={onPinAgent}
				onUnpinAgent={onUnpinAgent}
				onReorderPinnedAgent={onReorderPinnedAgent}
				onBeforeNewAgent={onBeforeNewAgent}
				onOpenRenameDialog={onRenameTitle ? setChatPendingRename : undefined}
				isCreating={isCreating}
				isArchiving={isArchiving}
				archivingChatId={archivingChatId}
				regeneratingTitleChatIds={regeneratingTitleChatIds}
				isLoading={isLoading}
				loadError={loadError}
				onRetryLoad={onRetryLoad}
				hasNextPage={hasNextPage}
				onLoadMore={onLoadMore}
				isFetchingNextPage={isFetchingNextPage}
				archivedFilter={archivedFilter}
				onArchivedFilterChange={onArchivedFilterChange}
				onCollapse={onCollapse}
				activeChatId={activeChatId}
				isSettingsPanel={isSettingsPanel}
				isChatsActive={!activeChatId && sidebarView.panel === "chats"}
				location={location}
			/>
			<SettingsPanel
				isSettingsPanel={isSettingsPanel}
				settingsPanel={settingsPanel}
				settingsSection={settingsSection}
				showApiKeysItem={showApiKeysItem}
				isPersonalModelOverridesEnabled={isPersonalModelOverridesEnabled}
				isAdmin={isAdmin}
				location={location}
				onCollapse={onCollapse}
			/>
			{onRenameTitle && (
				<RenameChatDialog
					chat={chatPendingRename}
					onRename={onRenameTitle}
					onPropose={onProposeTitle}
					onOpenChange={(open) => {
						if (!open) setChatPendingRename(null);
					}}
				/>
			)}
		</div>
	);
};
