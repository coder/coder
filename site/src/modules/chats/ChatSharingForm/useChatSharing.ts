import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { isApiValidationError } from "#/api/errors";
import {
	chatACL,
	setChatGroupRole,
	setChatUserRole,
} from "#/api/queries/chats";
import type {
	Chat,
	ChatGroup,
	ChatRole,
	ChatUser,
	Group,
} from "#/api/typesGenerated";

// PendingShareTarget captures the original request the owner wanted to
// make so we can replay it after the confirmation modal resolves.
export type PendingShareTarget =
	| { kind: "user"; user: ChatUser; role: ChatRole }
	| { kind: "group"; group: ChatGroup | Group; role: ChatRole };

export type PendingConfirmation = {
	target: PendingShareTarget;
	requiresToolCalls: boolean;
	requiresAttachments: boolean;
	message?: string;
	detail?: string;
};

type ShareConfirmFlags = {
	toolCalls?: boolean;
	attachments?: boolean;
};

// extractConfirmRequirements inspects an API validation error and
// returns the set of confirm_share_* fields the server reported as
// missing, or null if the error is not a confirm-share gate.
const extractConfirmRequirements = (
	error: unknown,
): Pick<
	PendingConfirmation,
	"requiresToolCalls" | "requiresAttachments" | "message" | "detail"
> | null => {
	if (!isApiValidationError(error)) {
		return null;
	}
	const validations = error.response.data.validations ?? [];
	const requiresToolCalls = validations.some(
		(v) => v.field === "confirm_share_tool_calls",
	);
	const requiresAttachments = validations.some(
		(v) => v.field === "confirm_share_attachments",
	);
	if (!requiresToolCalls && !requiresAttachments) {
		return null;
	}
	return {
		requiresToolCalls,
		requiresAttachments,
		message: error.response.data.message,
		detail: error.response.data.detail,
	};
};

/**
 * Encapsulates all data fetching and mutations for chat ACL sharing.
 * Mirrors useWorkspaceSharing but uses ChatUser/ChatGroup shapes and
 * surfaces the confirm_share_* flow as a `pendingConfirmation` that
 * callers can render as a modal.
 */
export function useChatSharing(chat: Chat) {
	const queryClient = useQueryClient();
	const chatACLQuery = useQuery(chatACL(chat.id));

	const userMutation = useMutation(setChatUserRole(queryClient));
	const groupMutation = useMutation(setChatGroupRole(queryClient));

	const [pendingConfirmation, setPendingConfirmation] =
		useState<PendingConfirmation | null>(null);

	const runUserMutation = async (
		user: ChatUser,
		role: ChatRole,
		confirm?: ShareConfirmFlags,
	) => {
		try {
			await userMutation.mutateAsync({
				chatId: chat.id,
				userId: user.id,
				role,
				confirm,
			});
			setPendingConfirmation(null);
			return true;
		} catch (err) {
			const needs = extractConfirmRequirements(err);
			if (needs) {
				setPendingConfirmation({
					target: { kind: "user", user, role },
					...needs,
				});
				return false;
			}
			throw err;
		}
	};

	const runGroupMutation = async (
		group: ChatGroup | Group,
		role: ChatRole,
		confirm?: ShareConfirmFlags,
	) => {
		try {
			await groupMutation.mutateAsync({
				chatId: chat.id,
				groupId: group.id,
				role,
				confirm,
			});
			setPendingConfirmation(null);
			return true;
		} catch (err) {
			const needs = extractConfirmRequirements(err);
			if (needs) {
				setPendingConfirmation({
					target: { kind: "group", group, role },
					...needs,
				});
				return false;
			}
			throw err;
		}
	};

	const addUser = async (user: ChatUser) => runUserMutation(user, "read");
	const removeUser = async (user: ChatUser) => runUserMutation(user, "");
	const addGroup = async (group: ChatGroup | Group) =>
		runGroupMutation(group, "read");
	const removeGroup = async (group: ChatGroup | Group) =>
		runGroupMutation(group, "");

	const confirmPending = async (confirm: ShareConfirmFlags) => {
		if (!pendingConfirmation) {
			return;
		}
		const { target } = pendingConfirmation;
		if (target.kind === "user") {
			await runUserMutation(target.user, target.role, confirm);
			return;
		}
		await runGroupMutation(target.group, target.role, confirm);
	};

	const cancelPending = () => setPendingConfirmation(null);

	const mutationError =
		pendingConfirmation !== null
			? undefined
			: (userMutation.error ?? groupMutation.error);

	return {
		chatACL: chatACLQuery.data,
		isLoading: chatACLQuery.isLoading,
		error: chatACLQuery.error,
		mutationError,
		addUser,
		removeUser,
		addGroup,
		removeGroup,
		isMutating: userMutation.isPending || groupMutation.isPending,
		updatingUserId: userMutation.isPending
			? userMutation.variables?.userId
			: undefined,
		updatingGroupId: groupMutation.isPending
			? groupMutation.variables?.groupId
			: undefined,
		pendingConfirmation,
		confirmPending,
		cancelPending,
	} as const;
}
