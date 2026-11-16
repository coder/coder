import { PinIcon } from "lucide-react";
import { useInfiniteQuery } from "react-query";
import {
	type ChatListInput,
	infiniteProjectFolderChats,
} from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { ChatTreeContext, useChatTree } from "../tree/ChatTreeContext";
import { buildChatTree, collectVisibleChatIDs } from "../tree/chatTree";
import { DraggableChatTreeNode } from "../tree/DraggableChatTreeNode";
import { LoadMoreSentinel } from "./LoadMoreSentinel";
import {
	countProjectFolderPins,
	mergeProjectFolderChats,
} from "./projectGrouping";

type ProjectFolderChatsProps = {
	readonly projectId: string;
	readonly projectLabel: string;
	readonly filedChats: readonly Chat[];
	readonly filedPinnedChats: readonly Chat[];
	readonly chatListInput: ChatListInput;
	readonly emptyMessage: string;
	/** Called when the focused Retry button goes away, as after a successful retry. */
	readonly onRetryFocusLost: () => void;
};

/** Fetches on its own, since the sidebar's loaded pages may not reach a project's older chats. */
export const ProjectFolderChats: React.FC<ProjectFolderChatsProps> = ({
	projectId,
	projectLabel,
	filedChats,
	filedPinnedChats,
	chatListInput,
	emptyMessage,
	onRetryFocusLost,
}) => {
	const chatTreeContext = useChatTree();
	const folderQuery = useInfiniteQuery(
		infiniteProjectFolderChats(projectId, chatListInput),
	);
	const fetchedChats = folderQuery.data?.pages.flat() ?? [];
	const chats = mergeProjectFolderChats({
		projectId,
		filedChats,
		fetchedChats,
		sidebarChatById: chatTreeContext.chatById,
		currentUserId: chatTreeContext.currentUserId,
		archived: chatListInput.archived ?? false,
	});
	const pinnedCount = countProjectFolderPins({
		projectId,
		filedPinnedChats,
		fetchedChats,
		sidebarChatById: chatTreeContext.chatById,
		currentUserId: chatTreeContext.currentUserId,
	});
	// Fetched chats are missing from the panel's tree, which rows read
	// their subagents from.
	const chatTree = buildChatTree(chats);
	const isEmpty = chats.length === 0 && pinnedCount === 0;
	// React runs ref cleanups before it removes the node, so focus has not
	// fallen to the page body yet. A re-render runs the cleanup too, but
	// leaves the node connected.
	const retryButtonRef = (node: HTMLButtonElement) => () => {
		if (!node.contains(document.activeElement)) {
			return;
		}
		queueMicrotask(() => {
			if (!node.isConnected && document.activeElement === document.body) {
				onRetryFocusLost();
			}
		});
	};

	return (
		<ChatTreeContext
			value={{
				...chatTreeContext,
				chatTree,
				chatById: chatTree.chatById,
				visibleChatIDs: collectVisibleChatIDs({
					chats,
					search: "",
					tree: chatTree,
				}),
			}}
		>
			{pinnedCount > 0 && (
				<p className="m-0 flex items-center gap-1.5 px-2 py-1 text-xs text-content-secondary">
					<PinIcon aria-hidden="true" className="size-3 shrink-0" />
					<span className="truncate">
						{pinnedCount} pinned {pinnedCount === 1 ? "chat" : "chats"}
					</span>
				</p>
			)}
			{chats.map((chat) => (
				<DraggableChatTreeNode key={chat.id} chat={chat} />
			))}
			{chats.length === 0 && folderQuery.isLoading && (
				<Skeleton className="mx-2 my-1.5 h-3.5 w-24" />
			)}
			{isEmpty && folderQuery.isSuccess && !folderQuery.hasNextPage && (
				<p className="m-0 px-2 py-1 text-xs text-content-secondary">
					{emptyMessage}
				</p>
			)}
			{folderQuery.isError ? (
				<ErrorAlert
					error={folderQuery.error}
					className="my-1"
					actions={
						<Button
							ref={retryButtonRef}
							size="sm"
							variant="outline"
							aria-label={`Retry loading chats in ${projectLabel}`}
							onClick={() => void folderQuery.refetch()}
						>
							Retry
						</Button>
					}
				/>
			) : (
				folderQuery.hasNextPage && (
					<LoadMoreSentinel
						onLoadMore={() => void folderQuery.fetchNextPage()}
						isFetchingNextPage={folderQuery.isFetchingNextPage}
					/>
				)
			)}
		</ChatTreeContext>
	);
};
