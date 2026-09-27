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
	getParkedWorkspaceUploads,
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
	const { uploads, attach, remove, uploadQueued } = useWorkspaceFileUploads(
		undefined,
		undefined,
		getParkedWorkspaceUploads(chatId),
	);
	const [failedUploads, setFailedUploads] = useState<
		readonly WorkspaceFileUpload[]
	>([]);

	const { isPending: isBatchPending, mutate: uploadBatch } = useMutation({
		mutationFn: async () => {
			const results = await uploadQueued(chatId);
			// Files parked while this batch ran stay for the next one.
			for (const upload of results) {
				remove(upload.id);
			}
			unparkWorkspaceUploads(
				chatId,
				results.map((upload) => upload.file),
			);
			const failed = results.filter(
				(upload) => upload.status !== "uploaded" || !upload.response,
			);
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
			if (content.length > 0) {
				await sendMessage({
					chatId,
					req: { content, busy_behavior: "queue" },
				});
			}
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
		if (failedUploads.some((upload) => upload.id === id)) {
			setFailedUploads((current) =>
				current.filter((upload) => upload.id !== id),
			);
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
		if (!upload) {
			return;
		}
		setFailedUploads((current) => current.filter((entry) => entry.id !== id));
		attachParked([upload.file]);
	};

	return {
		uploads: [...uploads, ...failedUploads],
		attach: attachParked,
		remove: removeParked,
		retry,
	};
}
