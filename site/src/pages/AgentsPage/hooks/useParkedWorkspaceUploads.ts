import { useEffect, useRef } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createChatMessageByChatId } from "#/api/queries/chats";
import { isAbortError } from "../utils/chatAttachments";
import { toWorkspaceFileReferencePart } from "../utils/chatInputContent";
import {
	clearParkedWorkspaceUploads,
	getParkedWorkspaceUploads,
	parkWorkspaceUploads,
} from "../utils/parkedWorkspaceUploads";
import {
	useWorkspaceFileUploads,
	type WorkspaceFileUpload,
} from "./useWorkspaceFileUploads";

/**
 * Uploads workspace files parked by the new-chat page once the chat's
 * workspace agent connects, then delivers them to the agent as a queued
 * follow-up message. The first message already started the turn that
 * creates or starts the workspace, so the files cannot ride along with it.
 */
export function useParkedWorkspaceUploads(
	chatId: string,
	canUpload: boolean,
): {
	uploads: readonly WorkspaceFileUpload[];
	remove: (id: string) => void;
} {
	const queryClient = useQueryClient();
	const sendMutation = useMutation(createChatMessageByChatId(queryClient));
	const parked = useWorkspaceFileUploads(undefined, undefined);
	const { uploads, attach, remove, reset, uploadQueued } = parked;
	const seededChatIdRef = useRef<string | null>(null);
	const runningRef = useRef(false);

	useEffect(() => {
		if (seededChatIdRef.current === chatId) {
			return;
		}
		seededChatIdRef.current = chatId;
		runningRef.current = false;
		reset();
		const files = getParkedWorkspaceUploads(chatId);
		if (files.length > 0) {
			attach([...files]);
		}
	}, [chatId, attach, reset]);

	useEffect(() => {
		if (
			!canUpload ||
			runningRef.current ||
			!uploads.some((upload) => upload.status === "deferred")
		) {
			return;
		}
		runningRef.current = true;
		const uploadChatId = chatId;
		void (async () => {
			let results: readonly WorkspaceFileUpload[];
			try {
				results = await uploadQueued(uploadChatId);
			} catch (error) {
				// Unmount or a chat switch canceled the batch. The parked
				// files stay registered so the next visit retries them.
				runningRef.current = false;
				if (!isAbortError(error)) {
					toast.error(
						getErrorMessage(error, "Failed to upload files to the workspace."),
					);
				}
				return;
			}
			clearParkedWorkspaceUploads(uploadChatId);
			reset();
			const failed = results.filter(
				(upload) => upload.status !== "uploaded" || !upload.response,
			);
			if (failed.length > 0) {
				toast.error(
					`Failed to upload to the workspace: ${failed.map((upload) => upload.file.name).join(", ")}`,
				);
			}
			const content = results.flatMap((upload) =>
				upload.response
					? [
							toWorkspaceFileReferencePart({
								path: upload.response.path,
								name: upload.response.name,
								size: upload.response.size,
								mediaType: upload.response.media_type,
								workspaceId: upload.response.workspace_id,
							}),
						]
					: [],
			);
			if (content.length === 0) {
				return;
			}
			sendMutation.mutate(
				{
					chatId: uploadChatId,
					req: { content, busy_behavior: "queue" },
				},
				{
					onError: (error) => {
						toast.error(
							getErrorMessage(
								error,
								"Uploaded the files to the workspace but failed to send them to the agent.",
							),
						);
					},
				},
			);
		})();
	}, [canUpload, chatId, uploads, uploadQueued, reset, sendMutation]);

	const removeParked = (id: string) => {
		remove(id);
		const remaining = uploads
			.filter((upload) => upload.id !== id)
			.map((upload) => upload.file);
		clearParkedWorkspaceUploads(chatId);
		parkWorkspaceUploads(chatId, remaining);
	};

	return { uploads, remove: removeParked };
}
