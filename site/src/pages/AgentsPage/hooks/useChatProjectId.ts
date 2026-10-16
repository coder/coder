import { useQuery } from "react-query";
import { chat as chatQuery } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";

type ChatProjectId = {
	/** The project whose skills apply to the chat, if any. */
	projectId: string | undefined;
	/**
	 * False while the project is unknown, so callers can avoid treating
	 * project skills as absent before the root chat loads. A failed root
	 * chat lookup resolves to no project so built-in commands keep working.
	 */
	isResolved: boolean;
};

/**
 * Resolves the project a chat belongs to. Subagent chats have no
 * project_id of their own and inherit the root chat's project, matching
 * chatd's project resolution.
 */
export const useChatProjectId = (
	chat: Chat | undefined,
	enabled: boolean,
): ChatProjectId => {
	const rootChatId = chat?.root_chat_id;
	const needsRootChat = Boolean(
		enabled && chat && !chat.project_id && rootChatId && rootChatId !== chat.id,
	);
	const rootChatQuery = useQuery({
		...chatQuery(rootChatId ?? ""),
		enabled: needsRootChat,
	});

	if (!enabled) {
		return { projectId: undefined, isResolved: true };
	}
	if (!needsRootChat) {
		return { projectId: chat?.project_id, isResolved: true };
	}
	return {
		projectId: rootChatQuery.data?.project_id,
		isResolved: rootChatQuery.data !== undefined || rootChatQuery.isError,
	};
};
