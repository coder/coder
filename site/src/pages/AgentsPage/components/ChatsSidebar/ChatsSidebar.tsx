import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { useLocation, useParams } from "react-router";
import { userChatProviderConfigs } from "#/api/queries/chats";
import type { Chat, ChatModelConfig } from "#/api/typesGenerated";
import { isMac } from "#/utils/platform";
import type { ModelSelectorOption } from "../ChatElements";
import { ChatsPanel } from "./chats/ChatsPanel";
import { ChatSearchDialog, RenameChatDialog } from "./dialogs";
import { SettingsPanel } from "./settings/SettingsPanel";
import { isSettingsView, sidebarViewFromPath } from "./sidebarView";

export { isSettingsView, sidebarViewFromPath } from "./sidebarView";

interface ChatsSidebarProps {
	chats: readonly Chat[];
	chatErrorReasons: Record<string, string>;
	modelOptions: readonly ModelSelectorOption[];
	modelConfigs: readonly ChatModelConfig[];
	onArchiveAgent: (chatId: string) => void;
	onUnarchiveAgent: (chatId: string) => void;
	onArchiveAndDeleteWorkspace: (chatId: string, workspaceId: string) => void;
	onPinAgent: (chatId: string) => void;
	onUnpinAgent: (chatId: string) => void;
	onReorderPinnedAgent?: (chatId: string, pinOrder: number) => void;
	onRenameTitle?: (chatId: string, title: string) => Promise<void>;
	onProposeTitle?: (chatId: string) => Promise<string>;
	onBeforeNewAgent?: () => void;
	isCreating: boolean;
	isArchiving?: boolean;
	archivingChatId?: string | null;
	regeneratingTitleChatIds: readonly string[];
	isLoading?: boolean;
	loadError?: unknown;
	onRetryLoad?: () => void;
	hasNextPage?: boolean;
	onLoadMore?: () => void;
	isFetchingNextPage?: boolean;
	archivedFilter: "active" | "archived";
	onArchivedFilterChange?: (filter: "active" | "archived") => void;
	onCollapse?: () => void;
	isPersonalModelOverridesEnabled?: boolean;
	isAdmin?: boolean;
}

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
	const [isSearchDialogOpen, setIsSearchDialogOpen] = useState(false);

	useEffect(() => {
		const handleKeyDown = (event: KeyboardEvent) => {
			const target = event.target as HTMLElement | null;
			if (target) {
				const tag = target.tagName;
				if (tag === "INPUT" || tag === "TEXTAREA" || target.isContentEditable) {
					return;
				}
			}

			const isModifierPressed = isMac() ? event.metaKey : event.ctrlKey;

			if (
				!isModifierPressed ||
				event.altKey ||
				event.shiftKey ||
				event.key.toLowerCase() !== "k"
			) {
				return;
			}

			event.preventDefault();
			setIsSearchDialogOpen(true);
		};

		document.addEventListener("keydown", handleKeyDown);
		return () => document.removeEventListener("keydown", handleKeyDown);
	}, []);

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
				onOpenSearchDialog={() => setIsSearchDialogOpen(true)}
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
			<ChatSearchDialog
				open={isSearchDialogOpen}
				onOpenChange={setIsSearchDialogOpen}
				location={location}
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
