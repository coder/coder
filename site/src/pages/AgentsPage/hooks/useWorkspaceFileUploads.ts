import { useEffect, useRef, useState } from "react";
import { useMutation } from "react-query";
import { API } from "#/api/api";
import type { UploadChatWorkspaceFileResponse } from "#/api/typesGenerated";
import { renameChatFileForUpload } from "../utils/chatAttachments";
import { formatAgentAttachmentUploadError } from "../utils/fileAttachmentLimits";

type WorkspaceFileUploadStatus = "queued" | "uploading" | "uploaded" | "error";

export type WorkspaceFileUpload = {
	id: string;
	file: File;
	status: WorkspaceFileUploadStatus;
	error?: string;
	// Set once status is "uploaded". Carries the final path, name,
	// size, and media type reported by the workspace agent.
	response?: UploadChatWorkspaceFileResponse;
};

export const isWorkspaceUploadInProgress = (
	upload: WorkspaceFileUpload,
): boolean => upload.status === "queued" || upload.status === "uploading";

type UseWorkspaceFileUploadsReturn = {
	uploads: readonly WorkspaceFileUpload[];
	attach: (files: File[]) => void;
	remove: (id: string) => void;
	reset: () => void;
};

// Workspace uploads have no size cap, so bound the number of parallel
// streams instead of relying on the browser's per-host connection
// limits alone.
const maxConcurrentWorkspaceUploads = 3;

const createUploadId = (): string => {
	const cryptoObject =
		typeof globalThis.crypto !== "undefined" ? globalThis.crypto : undefined;
	if (cryptoObject?.randomUUID) {
		return cryptoObject.randomUUID();
	}
	return `upload-${Date.now()}-${Math.random().toString(36).slice(2)}`;
};

/**
 * Manages eager uploads of files into the chat's workspace filesystem
 * via POST /api/v2/chats/{chat}/workspace-files.
 *
 * Uploads start as soon as files are attached, bounded to a small
 * number of concurrent streams. Removing an entry aborts its in-flight
 * upload, but bytes that already reached the workspace stay there;
 * removal only drops the composer reference.
 *
 * Uploads are scoped to the chat and its bound workspace: switching
 * either one resets the pending set, because already-uploaded bytes
 * live in the previous workspace and their references would be
 * unreadable for the new one.
 */
export function useWorkspaceFileUploads(
	chatId: string | undefined,
	workspaceId: string | undefined,
): UseWorkspaceFileUploadsReturn {
	const [uploads, setUploads] = useState<readonly WorkspaceFileUpload[]>([]);
	const abortControllersRef = useRef(new Map<string, AbortController>());
	const pendingQueueRef = useRef<{ id: string; file: File }[]>([]);
	const activeCountRef = useRef(0);
	// Incremented whenever pending uploads are aborted (reset or scope
	// switch). Workers spawned under an older generation must not
	// consume entries queued after the abort: they carry a stale chat
	// ID and their slot accounting was already zeroed.
	const generationRef = useRef(0);
	const scopeKey = `${chatId ?? ""}/${workspaceId ?? ""}`;
	const previousScopeKeyRef = useRef(scopeKey);

	const uploadMutation = useMutation({
		mutationFn: ({
			uploadChatId,
			file,
			signal,
		}: {
			uploadChatId: string;
			file: File;
			signal: AbortSignal;
		}) => API.experimental.uploadChatWorkspaceFile(uploadChatId, file, signal),
	});
	const { mutateAsync: uploadFile } = uploadMutation;

	const abortAllUploads = () => {
		generationRef.current++;
		for (const controller of abortControllersRef.current.values()) {
			controller.abort();
		}
		abortControllersRef.current.clear();
		pendingQueueRef.current = [];
		activeCountRef.current = 0;
	};

	const reset = () => {
		abortAllUploads();
		setUploads([]);
	};

	// Abort any in-flight uploads when the composer unmounts.
	useEffect(() => abortAllUploads, [abortAllUploads]);

	// Uploads target a specific chat's directory in a specific
	// workspace, so navigating to a different chat or rebinding the
	// workspace invalidates the pending set.
	useEffect(() => {
		if (previousScopeKeyRef.current === scopeKey) {
			return;
		}
		previousScopeKeyRef.current = scopeKey;
		reset();
	}, [scopeKey, reset]);

	const setUploadResult = (
		id: string,
		result: Partial<WorkspaceFileUpload>,
	) => {
		setUploads((prev) =>
			prev.map((upload) =>
				upload.id === id ? { ...upload, ...result } : upload,
			),
		);
	};

	// Each worker pulls queued files until the queue drains, so a
	// completed upload immediately frees its slot for the next file.
	const runUploadWorker = async (uploadChatId: string) => {
		const generation = generationRef.current;
		activeCountRef.current++;
		let next = pendingQueueRef.current.shift();
		while (next) {
			// A removed entry has no abort controller anymore; skip it.
			const controller = abortControllersRef.current.get(next.id);
			if (controller) {
				setUploadResult(next.id, { status: "uploading" });
				try {
					const response = await uploadFile({
						uploadChatId,
						file: next.file,
						signal: controller.signal,
					});
					setUploadResult(next.id, { status: "uploaded", response });
				} catch (error: unknown) {
					if (!controller.signal.aborted) {
						setUploadResult(next.id, {
							status: "error",
							error: formatAgentAttachmentUploadError(error),
						});
					}
				}
				abortControllersRef.current.delete(next.id);
			}
			if (generationRef.current !== generation) {
				// An abort invalidated this worker mid-upload. Entries
				// queued since belong to workers of the new generation,
				// which also owns the slot accounting.
				return;
			}
			next = pendingQueueRef.current.shift();
		}
		activeCountRef.current--;
	};

	const pumpQueue = () => {
		if (!chatId) {
			return;
		}
		// Workers shift their first queue entry synchronously, so this
		// loop spawns at most one worker per pending file.
		while (
			activeCountRef.current < maxConcurrentWorkspaceUploads &&
			pendingQueueRef.current.length > 0
		) {
			void runUploadWorker(chatId);
		}
	};

	const attach = (incoming: File[]) => {
		const entries = incoming.map((file) => ({
			id: createUploadId(),
			file: renameChatFileForUpload(file),
			status: "queued" as const,
		}));
		if (!chatId) {
			setUploads((prev) => [
				...prev,
				...entries.map((entry) => ({
					...entry,
					status: "error" as const,
					error: "Cannot upload: no active chat. Open or start a chat first.",
				})),
			]);
			return;
		}
		setUploads((prev) => [...prev, ...entries]);
		for (const entry of entries) {
			abortControllersRef.current.set(entry.id, new AbortController());
			pendingQueueRef.current.push({ id: entry.id, file: entry.file });
		}
		pumpQueue();
	};

	const remove = (id: string) => {
		abortControllersRef.current.get(id)?.abort();
		abortControllersRef.current.delete(id);
		pendingQueueRef.current = pendingQueueRef.current.filter(
			(entry) => entry.id !== id,
		);
		setUploads((prev) => prev.filter((upload) => upload.id !== id));
	};

	return { uploads, attach, remove, reset };
}
