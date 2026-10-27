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
	/** Names the project in the Retry button's label. */
	readonly projectLabel: string;
	/** The loaded sidebar chats filed under this project. */
	readonly filedChats: readonly Chat[];
	/** The loaded sidebar pins in this project. */
	readonly filedPinnedChats: readonly Chat[];
	readonly chatListInput: ChatListInput;
	readonly emptyMessage: string;
};

/**
 * An expanded folder's chats. The folder fetches its project's chats itself,
 * because the sidebar's loaded pages may not reach the older ones.
 */
export const ProjectFolderChats: React.FC<ProjectFolderChatsProps> = ({
	projectId,
	projectLabel,
	filedChats,
	filedPinnedChats,
	chatListInput,
	emptyMessage,
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
		currentUserId: chatTreeContext.currentUserId,
		archived: chatListInput.archived ?? false,
	});
	const pinnedCount = countProjectFolderPins({
		projectId,
		filedPinnedChats,
		fetchedChats,
		currentUserId: chatTreeContext.currentUserId,
	});
	// Fetched chats are missing from the panel's tree, which rows read
	// their subagents from.
	const chatTree = buildChatTree(chats);
	const isEmpty = chats.length === 0 && pinnedCount === 0;

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
						{pinnedCount} pinned {pinnedCount === 1 ? "chat" : "chats"}, shown
						in Pinned
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
