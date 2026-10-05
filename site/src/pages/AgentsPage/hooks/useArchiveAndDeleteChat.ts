import { useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	archiveAndDeleteChat,
	archiveAndDeleteChatKey,
	chatArchiveMutationKey,
} from "#/api/queries/chats";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import type { Chat, Workspace } from "#/api/typesGenerated";
import {
	type ArchiveAndDeleteAction,
	fetchArchiveAndDeleteAction,
	notifyArchiveAndDeleteFailed,
	notifyDeleteQueueState,
} from "../utils/agentWorkspaceUtils";
import { clearPersistedRightPanelState } from "../utils/rightPanelTabStorage";
import { clearPersistedSidebarTabId } from "../utils/sidebarTabStorage";

/** Shares the archive-and-delete workflow between chat action menus. */
export const useArchiveAndDeleteChat = (
	chat: Chat | undefined,
	onArchived: ((chatId: string) => void) | undefined,
) => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const [confirmation, setConfirmation] = useState<{
		chatId: string;
		workspace: Workspace;
	}>();
	const filters = { mutationKey: chatArchiveMutationKey(chat?.id ?? "") };
	const options = archiveAndDeleteChat(queryClient);
	const mutation = useMutation({
		...options,
		mutationKey: archiveAndDeleteChatKey(chat?.id ?? ""),
		onSuccess: (result, variables) => {
			options.onSuccess(result, variables);
			clearPersistedSidebarTabId(variables.chatId);
			clearPersistedRightPanelState(variables.chatId);
			if (variables.workspaceId) {
				notifyDeleteQueueState(
					queryClient.getQueryData<Workspace>(
						workspaceByIdKey(variables.workspaceId),
					),
					result.deleteBuild,
				);
			}
			onArchived?.(variables.chatId);
		},
		onError: (error, variables) => {
			notifyArchiveAndDeleteFailed(
				variables.workspaceId
					? queryClient.getQueryData<Workspace>(
							workspaceByIdKey(variables.workspaceId),
						)
					: undefined,
				error,
				navigate,
			);
		},
	});

	const requestArchiveAndDelete = async () => {
		const workspaceId = chat?.workspace_id;
		if (!chat || !workspaceId || queryClient.isMutating(filters)) {
			return;
		}
		let action: ArchiveAndDeleteAction;
		try {
			action = await fetchArchiveAndDeleteAction(
				queryClient,
				workspaceId,
				chat.created_at,
			);
		} catch (error) {
			toast.error(
				getErrorMessage(error, "Failed to look up workspace for deletion."),
			);
			return;
		}
		if (queryClient.isMutating(filters)) {
			return;
		}
		if (action === "confirm") {
			const workspace = queryClient.getQueryData<Workspace>(
				workspaceByIdKey(workspaceId),
			);
			if (workspace) {
				setConfirmation({ chatId: chat.id, workspace });
			}
		} else {
			mutation.mutate({
				chatId: chat.id,
				workspaceId: action === "archive-only" ? undefined : workspaceId,
			});
		}
	};

	return {
		requestArchiveAndDelete,
		dialogProps: {
			workspace: confirmation?.workspace,
			onCancel: () => setConfirmation(undefined),
			onConfirm: () => {
				if (
					confirmation &&
					!queryClient.isMutating({
						mutationKey: chatArchiveMutationKey(confirmation.chatId),
					})
				) {
					mutation.mutate({
						chatId: confirmation.chatId,
						workspaceId: confirmation.workspace.id,
					});
				}
				setConfirmation(undefined);
			},
		},
	};
};
