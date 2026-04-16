import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatACL,
	setChatGroupRole,
	setChatUserRole,
} from "#/api/queries/chats";
import type {
	Chat,
	ChatGroup,
	ChatShareEntry,
	ChatUser,
	Group,
} from "#/api/typesGenerated";

/**
 * Encapsulates all data fetching and mutations for chat ACL sharing.
 * Each mutation accepts a full ChatShareEntry so callers can set the
 * role and the per-entry share_tool_calls / share_attachments flags in
 * a single request.
 */
export function useChatSharing(chat: Chat) {
	const queryClient = useQueryClient();
	const chatACLQuery = useQuery(chatACL(chat.id));

	const userMutation = useMutation(setChatUserRole(queryClient));
	const groupMutation = useMutation(setChatGroupRole(queryClient));

	const setUserEntry = (user: ChatUser, entry: ChatShareEntry) =>
		userMutation.mutateAsync({ chatId: chat.id, userId: user.id, entry });

	const setGroupEntry = (group: ChatGroup | Group, entry: ChatShareEntry) =>
		groupMutation.mutateAsync({ chatId: chat.id, groupId: group.id, entry });

	const addUser = (user: ChatUser) =>
		setUserEntry(user, {
			role: "read",
			share_tool_calls: false,
			share_attachments: false,
		});

	const removeUser = (user: ChatUser) => setUserEntry(user, { role: "" });

	const addGroup = (group: ChatGroup | Group) =>
		setGroupEntry(group, {
			role: "read",
			share_tool_calls: false,
			share_attachments: false,
		});

	const removeGroup = (group: ChatGroup | Group) =>
		setGroupEntry(group, { role: "" });

	return {
		chatACL: chatACLQuery.data,
		isLoading: chatACLQuery.isLoading,
		error: chatACLQuery.error,
		mutationError: userMutation.error ?? groupMutation.error,
		addUser,
		removeUser,
		addGroup,
		removeGroup,
		setUserEntry,
		setGroupEntry,
		isMutating: userMutation.isPending || groupMutation.isPending,
		updatingUserId: userMutation.isPending
			? userMutation.variables?.userId
			: undefined,
		updatingGroupId: groupMutation.isPending
			? groupMutation.variables?.groupId
			: undefined,
	} as const;
}
