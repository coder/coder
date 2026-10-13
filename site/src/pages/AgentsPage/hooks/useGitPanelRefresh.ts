import { useQueryClient } from "react-query";
import {
	invalidateChatDiffContents,
	invalidateChatEntity,
} from "#/api/queries/chats";

/**
 * Returns the git panel's refresh handler, so refresh shows what a page
 * reload would. It refetches the chat, whose diff statuses back the PR
 * tabs, and the remote diff contents. Both come from coderd and do not
 * need the workspace agent, so they are refetched even when the git
 * watch socket is down. The handler then asks the git watcher to rescan
 * and returns whether that request was sent.
 */
export const useGitPanelRefresh = (
	chatId: string,
	refreshGitWatcher: () => boolean,
): (() => boolean) => {
	const queryClient = useQueryClient();
	return () => {
		void invalidateChatEntity(queryClient, chatId);
		void invalidateChatDiffContents(queryClient, chatId);
		return refreshGitWatcher();
	};
};
