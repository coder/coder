import { useEffect, useRef, useState } from "react";
import { useMutation } from "react-query";
import { API } from "#/api/api";
import type { UploadChatWorkspaceFileResponse } from "#/api/typesGenerated";
import { renameChatFileForUpload } from "../utils/chatAttachments";
import { formatAgentAttachmentUploadError } from "../utils/fileAttachmentLimits";

type WorkspaceFileUploadStatus =
	| "deferred"
	| "queued"
	| "uploading"
	| "uploaded"
	| "error";

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

const createUploadCancellationError = (): Error => {
	const error = new Error("Workspace file upload canceled.");
	error.name = "AbortError";
	return error;
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
	const uploadsRef = useRef<readonly WorkspaceFileUpload[]>([]);
	const generationRef = useRef(0);
	// Deferred callbacks capture this render's generation. A reset
	// updates the state after incrementing the ref, so callbacks from
	// an older render reject instead of operating on the new queue.
	const [queueGeneration, setQueueGeneration] = useState(0);
	const abortControllersRef = useRef(new Map<string, AbortController>());
	const pendingQueueRef = useRef<{ id: string; file: File }[]>([]);
	const activeCountRef = useRef(0);
	// Per-entry settle callbacks for uploadQueued. Explicit removal
	// resolves with null; reset and scope cancellation reject the batch.
	const settleCallbacksRef = useRef(
		new Map<
			string,
			{
				resolve: (result: WorkspaceFileUpload | null) => void;
				reject: (error: Error) => void;
			}
		>(),
	);
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

	const updateUploads = (
		update: (
			current: readonly WorkspaceFileUpload[],
		) => readonly WorkspaceFileUpload[],
	) => {
		const next = update(uploadsRef.current);
		uploadsRef.current = next;
		setUploads(next);
	};

	const settleUpload = (id: string, result: WorkspaceFileUpload | null) => {
		const settle = settleCallbacksRef.current.get(id);
		if (settle) {
			settleCallbacksRef.current.delete(id);
			settle.resolve(result);
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
		const cancellationError = createUploadCancellationError();
		for (const settle of settleCallbacksRef.current.values()) {
			settle.reject(cancellationError);
		}
		settleCallbacksRef.current.clear();
		removedIdsRef.current.clear();
	};

	const reset = () => {
		abortAllUploads();
		setQueueGeneration(generationRef.current);
		updateUploads(() => []);
	};

	// Abort any in-flight uploads when the composer unmounts. StrictMode
	// replays this cleanup on a simulated unmount and keeps the
	// component, so the generation state must follow the ref or every
	// later deferred callback would reject as stale.
	useEffect(
		() => () => {
			abortAllUploads();
			setQueueGeneration(generationRef.current);
		},
		[abortAllUploads],
	);

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
		updateUploads((current) =>
			current.map((upload) =>
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
			updateUploads((current) => [
				...current,
				...entries.map((entry) => ({ ...entry, status: "deferred" as const })),
			]);
			return;
		}
		updateUploads((current) => [
			...current,
			...entries.map((entry) => ({
				...entry,
				status: "queued" as const,
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
		if (generationRef.current !== queueGeneration) {
			throw createUploadCancellationError();
		}
		const entries = uploadsRef.current;
		if (entries.length === 0) {
			return [];
		}
		const settled = entries.map(
			(entry) =>
				new Promise<WorkspaceFileUpload | null>((resolve, reject) => {
					settleCallbacksRef.current.set(entry.id, { resolve, reject });
				}),
		);
		updateUploads((current) =>
			current.map((upload) => ({
				id: upload.id,
				file: upload.file,
				status: "queued" as const,
			})),
		);
		for (const entry of entries) {
			abortControllersRef.current.set(entry.id, new AbortController());
			pendingQueueRef.current.push({ id: entry.id, file: entry.file });
		}
		pumpQueue(uploadChatId);
		const results = await Promise.all(settled);
		if (generationRef.current !== queueGeneration) {
			throw createUploadCancellationError();
		}
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
		updateUploads((current) => current.filter((upload) => upload.id !== id));
	};

	return { uploads, attach, remove, reset, uploadQueued };
}
