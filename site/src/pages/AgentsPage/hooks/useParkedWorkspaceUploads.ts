import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createChatMessageByChatId } from "#/api/queries/chats";
import {
	isAbortError,
	renameChatFileForUpload,
} from "../utils/chatAttachments";
import { toWorkspaceFileReferencePart } from "../utils/chatInputContent";
import {
	clearParkedWorkspaceUploadFailure,
	getParkedWorkspaceUploads,
	markParkedWorkspaceUploadFailed,
	parkWorkspaceUploads,
	unparkWorkspaceUploads,
} from "../utils/parkedWorkspaceUploads";
import {
	useWorkspaceFileUploads,
	type WorkspaceFileUpload,
} from "./useWorkspaceFileUploads";

type UseParkedWorkspaceUploadsReturn = {
	uploads: readonly WorkspaceFileUpload[];
	attach: (files: File[]) => void;
	remove: (id: string) => void;
	retry: (id: string) => void;
};

/**
 * Holds workspace files attached while the chat has no running workspace,
 * uploads them once the chat's workspace agent connects, then delivers
 * them to the agent as a queued follow-up message. They cannot ride along
 * with an earlier message: that message's turn is what creates or starts
 * the workspace. Files already parked for the chat are read on mount, so
 * callers remount the hook per chat.
 */
export function useParkedWorkspaceUploads(
	chatId: string,
	canUpload: boolean,
): UseParkedWorkspaceUploadsReturn {
	const queryClient = useQueryClient();
	const { mutateAsync: sendMessage } = useMutation(
		createChatMessageByChatId(queryClient),
	);
	const parkedUploads = getParkedWorkspaceUploads(chatId);
	const { uploads, attach, remove, uploadQueued } = useWorkspaceFileUploads(
		undefined,
		undefined,
		parkedUploads.flatMap((upload) =>
			upload.failedUpload ? [] : [upload.file],
		),
	);
	const [failedUploads, setFailedUploads] = useState(() =>
		parkedUploads.flatMap((upload) =>
			upload.failedUpload ? [upload.failedUpload] : [],
		),
	);

	const { isPending: isBatchPending, mutate: uploadBatch } = useMutation({
		mutationFn: async () => {
			const results = await uploadQueued(chatId);
			// Files parked while this batch ran stay for the next one.
			const uploaded = results.filter((upload) => upload.response);
			const failed = results.filter((upload) => !upload.response);
			for (const upload of failed) {
				remove(upload.id);
				markParkedWorkspaceUploadFailed(chatId, upload);
			}
			if (failed.length > 0) {
				setFailedUploads((current) => [...current, ...failed]);
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
			// Chips stay until the follow-up is sent, so they do not disappear
			// before the message shows up in the chat.
			await sendMessage({
				chatId,
				req: { content, busy_behavior: "queue" },
			}).finally(() => {
				for (const upload of uploaded) {
					remove(upload.id);
				}
				unparkWorkspaceUploads(
					chatId,
					uploaded.map((upload) => upload.file),
				);
			});
		},
		onError: (error) => {
			// Unmounting cancels the batch, and its files stay parked for
			// the next visit.
			if (!isAbortError(error)) {
				toast.error(
					getErrorMessage(
						error,
						"Uploaded the files to the workspace but failed to send them to the agent.",
					),
				);
			}
		},
	});

	const hasDeferredUploads = uploads.some(
		(upload) => upload.status === "deferred",
	);
	// Parked files wait on an external event: the chat's workspace agent
	// connecting.
	useEffect(() => {
		if (canUpload && hasDeferredUploads && !isBatchPending) {
			uploadBatch();
		}
	}, [canUpload, hasDeferredUploads, isBatchPending, uploadBatch]);

	const attachParked = (files: File[]) => {
		// The store matches entries by File identity, so it must hold the
		// renamed objects the upload entries carry.
		const renamed = files.map(renameChatFileForUpload);
		attach(renamed);
		parkWorkspaceUploads(chatId, renamed);
	};

	const removeParked = (id: string) => {
		const failedUpload = failedUploads.find((upload) => upload.id === id);
		if (failedUpload) {
			setFailedUploads((current) =>
				current.filter((upload) => upload.id !== id),
			);
			unparkWorkspaceUploads(chatId, [failedUpload.file]);
			return;
		}
		const upload = uploads.find((entry) => entry.id === id);
		if (upload) {
			remove(id);
			unparkWorkspaceUploads(chatId, [upload.file]);
		}
	};

	const retry = (id: string) => {
		const upload = failedUploads.find((entry) => entry.id === id);
		// A second click can land before this render updates, so the store
		// decides whether the file still needs a retry.
		if (!upload || !clearParkedWorkspaceUploadFailure(chatId, upload.file)) {
			return;
		}
		setFailedUploads((current) => current.filter((entry) => entry.id !== id));
		attach([upload.file]);
	};

	return {
		uploads: [...uploads, ...failedUploads],
		attach: attachParked,
		remove: removeParked,
		retry,
	};
}
