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

type UseWorkspaceFileUploadsReturn = {
	uploads: readonly WorkspaceFileUpload[];
	attach: (files: File[]) => void;
	remove: (id: string) => void;
	reset: () => void;
	// Uploads every entry into the given chat and resolves with the
	// settled per-file results (entries removed mid-flight are
	// excluded). Used by the deferred mode, where files queue before
	// the chat exists. Prior results are discarded: a retry targets a
	// fresh chat, so paths uploaded for an abandoned one are unusable.
	// Must not run concurrently with itself.
	uploadQueued: (chatId: string) => Promise<readonly WorkspaceFileUpload[]>;
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
 * Manages uploads of files into the chat's workspace filesystem via
 * POST /api/v2/chats/{chat}/workspace-files.
 *
 * With a chat ID, uploads start eagerly as soon as files are attached,
 * bounded to a small number of concurrent streams. Without a chat ID,
 * attached files queue locally (deferred mode, used by the new-chat
 * page) until `uploadQueued` runs them against a just-created chat.
 * Removing an entry aborts its in-flight upload, but bytes that
 * already reached the workspace stay there; removal only drops the
 * composer reference.
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
	// Per-entry settle callbacks for uploadQueued. Entries resolve with
	// the final upload record, or null when removed/aborted mid-flight.
	const settleCallbacksRef = useRef(
		new Map<string, (result: WorkspaceFileUpload | null) => void>(),
	);
	// Incremented whenever pending uploads are aborted (reset or scope
	// switch). Workers spawned under an older generation must not
	// consume entries queued after the abort: they carry a stale chat
	// ID and their slot accounting was already zeroed.
	const generationRef = useRef(0);
	// Ids removed since the last reset. A chip removed after its file
	// already settled cannot un-resolve that settle promise, so
	// uploadQueued drops these ids from its final results instead.
	const removedIdsRef = useRef(new Set<string>());
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

	const settleUpload = (id: string, result: WorkspaceFileUpload | null) => {
		const settle = settleCallbacksRef.current.get(id);
		if (settle) {
			settleCallbacksRef.current.delete(id);
			settle(result);
		}
	};

	const abortAllUploads = () => {
		generationRef.current++;
		for (const controller of abortControllersRef.current.values()) {
			controller.abort();
		}
		abortControllersRef.current.clear();
		pendingQueueRef.current = [];
		activeCountRef.current = 0;
		// Settle any outstanding uploadQueued waiters so callers never
		// hang across a reset or scope switch.
		for (const settle of settleCallbacksRef.current.values()) {
			settle(null);
		}
		settleCallbacksRef.current.clear();
		removedIdsRef.current.clear();
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
				try {
					const response = await uploadFile({
						uploadChatId,
						file: next.file,
						signal: controller.signal,
					});
					setUploadResult(next.id, { status: "uploaded", response });
					settleUpload(next.id, {
						id: next.id,
						file: next.file,
						status: "uploaded",
						response,
					});
				} catch (error: unknown) {
					if (!controller.signal.aborted) {
						const message = formatAgentAttachmentUploadError(error);
						setUploadResult(next.id, {
							status: "error",
							error: message,
						});
						settleUpload(next.id, {
							id: next.id,
							file: next.file,
							status: "error",
							error: message,
						});
					} else {
						settleUpload(next.id, null);
					}
				}
				abortControllersRef.current.delete(next.id);
			} else {
				settleUpload(next.id, null);
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

	const pumpQueue = (uploadChatId: string) => {
		// Workers shift their first queue entry synchronously, so this
		// loop spawns at most one worker per pending file.
		while (
			activeCountRef.current < maxConcurrentWorkspaceUploads &&
			pendingQueueRef.current.length > 0
		) {
			void runUploadWorker(uploadChatId);
		}
	};

	const attach = (incoming: File[]) => {
		const entries = incoming.map((file) => ({
			id: createUploadId(),
			file: renameChatFileForUpload(file),
		}));
		if (!chatId) {
			// Deferred mode: no chat exists yet, so entries queue
			// locally until uploadQueued runs them.
			setUploads((prev) => [
				...prev,
				...entries.map((entry) => ({ ...entry, status: "queued" as const })),
			]);
			return;
		}
		setUploads((prev) => [
			...prev,
			...entries.map((entry) => ({
				...entry,
				status: "uploading" as const,
			})),
		]);
		for (const entry of entries) {
			abortControllersRef.current.set(entry.id, new AbortController());
			pendingQueueRef.current.push({ id: entry.id, file: entry.file });
		}
		pumpQueue(chatId);
	};

	const uploadQueued = async (
		uploadChatId: string,
	): Promise<readonly WorkspaceFileUpload[]> => {
		// Entry ids and files are immutable, so the render-time
		// snapshot is safe here even though statuses may be stale.
		const entries = uploads;
		if (entries.length === 0) {
			return [];
		}
		const settled = entries.map(
			(entry) =>
				new Promise<WorkspaceFileUpload | null>((resolve) => {
					settleCallbacksRef.current.set(entry.id, resolve);
				}),
		);
		setUploads((prev) =>
			prev.map((upload) => ({
				id: upload.id,
				file: upload.file,
				status: "uploading" as const,
			})),
		);
		for (const entry of entries) {
			abortControllersRef.current.set(entry.id, new AbortController());
			pendingQueueRef.current.push({ id: entry.id, file: entry.file });
		}
		pumpQueue(uploadChatId);
		const results = await Promise.all(settled);
		return results.filter(
			(result): result is WorkspaceFileUpload =>
				result !== null && !removedIdsRef.current.has(result.id),
		);
	};

	const remove = (id: string) => {
		removedIdsRef.current.add(id);
		abortControllersRef.current.get(id)?.abort();
		abortControllersRef.current.delete(id);
		pendingQueueRef.current = pendingQueueRef.current.filter(
			(entry) => entry.id !== id,
		);
		settleUpload(id, null);
		setUploads((prev) => prev.filter((upload) => upload.id !== id));
	};

	return { uploads, attach, remove, reset, uploadQueued };
}
