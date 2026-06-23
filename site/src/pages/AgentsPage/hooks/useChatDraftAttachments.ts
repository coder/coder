import { useEffect, useRef, useState } from "react";
import { API } from "#/api/api";
import { MaxChatFileSizeBytes } from "#/api/typesGenerated";
import type { UploadState } from "../components/AgentChatInput";
import {
	getChatFileURL,
	renameChatFileForUpload,
} from "../utils/chatAttachments";
import {
	clearChatDraftAttachmentRecords,
	fileToDataURL,
	type RestoredChatDraftAttachment,
	removeChatDraftAttachmentRecord,
	restoreChatDraftAttachments,
	upsertChatDraftAttachmentRecord,
} from "../utils/chatDraftAttachmentStorage";
import {
	formatAgentAttachmentTooLargeError,
	formatAgentAttachmentUploadError,
	readAgentAttachmentText,
} from "../utils/fileAttachmentLimits";
import {
	imageBudgetForProvider,
	imageNeedsResize,
	providerBudgetError,
} from "../utils/imageBudget";
import { resizeImageToMaxBytes } from "../utils/resizeImage";

const maxTextPreviewSize = 1024 * 1024;

const pendingDraftWarning =
	"This file is attached for now, but it could not be saved as a draft. If you leave this chat before it uploads or sends, it may be lost.";
const uploadedDraftWarning =
	"This file is usable in this session, but it could not be saved as a draft.";

type DraftUploadStatus = UploadState["status"];

type DraftAttachmentView = {
	clientId: string;
	file: File;
	fileId?: string;
	status: DraftUploadStatus;
	error?: string;
	draftWarning?: string;
	previewUrl?: string;
	previewUrlKind?: "blob" | "chatFile";
	textContent?: string;
};

type UploadRegistrySnapshot = {
	clientId: string;
	organizationId: string;
	chatId: string;
	file: File;
	fileId?: string;
	status: DraftUploadStatus;
	error?: string;
	draftWarning?: string;
	removed: boolean;
};

type UploadRegistrySubscriber = (snapshot: UploadRegistrySnapshot) => void;

type UploadRegistryEntry = {
	clientId: string;
	organizationId: string;
	chatId: string;
	file: File;
	generation: number;
	status: DraftUploadStatus;
	fileId?: string;
	error?: string;
	draftWarning?: string;
	removed: boolean;
	uploadStarted: boolean;
	subscribers: Set<UploadRegistrySubscriber>;
};

// Uploads outlive one chat page instance. The registry lets a remount rejoin
// an in-flight upload by clientId instead of starting a duplicate server
// upload. Async completions must check generation so removed drafts cannot
// write storage or notify UI again.
const activeDraftUploads = new Map<string, UploadRegistryEntry>();

let fallbackClientIdCounter = 0;

const isTerminalRegistryStatus = (entry: UploadRegistryEntry) =>
	entry.status === "uploaded" || entry.status === "error";

const pruneTerminalRegistryEntry = (entry: UploadRegistryEntry) => {
	if (entry.subscribers.size === 0 && isTerminalRegistryStatus(entry)) {
		activeDraftUploads.delete(entry.clientId);
	}
};

const createClientId = () => {
	const cryptoObject =
		typeof globalThis.crypto !== "undefined" ? globalThis.crypto : undefined;
	if (cryptoObject?.randomUUID) {
		return cryptoObject.randomUUID();
	}
	if (cryptoObject?.getRandomValues) {
		const values = new Uint32Array(2);
		cryptoObject.getRandomValues(values);
		return `draft-${Date.now()}-${Array.from(values, (value) => value.toString(36)).join("-")}`;
	}
	fallbackClientIdCounter += 1;
	return `draft-${Date.now()}-${fallbackClientIdCounter}`;
};

const createBlobPreview = (file: File): string | undefined => {
	if (file.type === "text/plain" || typeof URL.createObjectURL !== "function") {
		return undefined;
	}
	try {
		return URL.createObjectURL(file);
	} catch {
		return undefined;
	}
};

const revokeBlobPreview = (view: DraftAttachmentView) => {
	if (view.previewUrlKind === "blob" && view.previewUrl?.startsWith("blob:")) {
		URL.revokeObjectURL(view.previewUrl);
	}
};

type DraftAttachmentPreview = Pick<
	DraftAttachmentView,
	"previewUrl" | "previewUrlKind"
>;

const computePreview = (
	file: File,
	status: DraftUploadStatus,
	fileId?: string,
	current?: DraftAttachmentPreview,
): DraftAttachmentPreview => {
	if (status === "uploaded") {
		if (fileId && file.type.startsWith("image/")) {
			return { previewUrl: getChatFileURL(fileId), previewUrlKind: "chatFile" };
		}
		return {};
	}
	if (current) {
		return current;
	}
	const previewUrl = createBlobPreview(file);
	return { previewUrl, previewUrlKind: previewUrl ? "blob" : undefined };
};

const snapshotFromEntry = (
	entry: UploadRegistryEntry,
): UploadRegistrySnapshot => ({
	clientId: entry.clientId,
	organizationId: entry.organizationId,
	chatId: entry.chatId,
	file: entry.file,
	fileId: entry.fileId,
	status: entry.status,
	error: entry.error,
	draftWarning: entry.draftWarning,
	removed: entry.removed,
});

const notifySubscribers = (entry: UploadRegistryEntry) => {
	const snapshot = snapshotFromEntry(entry);
	for (const subscriber of entry.subscribers) {
		subscriber(snapshot);
	}
};

const isCurrentGeneration = (entry: UploadRegistryEntry, generation: number) =>
	!entry.removed && entry.generation === generation;

const createRegistryEntry = (
	clientId: string,
	organizationId: string,
	chatId: string,
	file: File,
): UploadRegistryEntry => {
	const existing = activeDraftUploads.get(clientId);
	if (existing) {
		return existing;
	}
	const entry: UploadRegistryEntry = {
		clientId,
		organizationId,
		chatId,
		file,
		generation: 1,
		status: "pending",
		removed: false,
		uploadStarted: false,
		subscribers: new Set(),
	};
	activeDraftUploads.set(clientId, entry);
	return entry;
};

const persistUploadPayload = async (
	entry: UploadRegistryEntry,
	generation: number,
) => {
	try {
		const payload = await fileToDataURL(entry.file);
		if (
			!isCurrentGeneration(entry, generation) ||
			entry.status === "uploaded"
		) {
			return;
		}
		const result = upsertChatDraftAttachmentRecord({
			status: entry.status === "pending" ? "pending" : "uploading",
			clientId: entry.clientId,
			fileName: entry.file.name,
			fileType: entry.file.type,
			lastModified: entry.file.lastModified,
			size: entry.file.size,
			organizationId: entry.organizationId,
			chatId: entry.chatId,
			payload,
		});
		if (result.ok || !isCurrentGeneration(entry, generation)) {
			return;
		}
		entry.draftWarning = pendingDraftWarning;
		notifySubscribers(entry);
	} catch {
		if (!isCurrentGeneration(entry, generation)) {
			return;
		}
		entry.draftWarning = pendingDraftWarning;
		notifySubscribers(entry);
	}
};

const persistUploadedRecord = (
	entry: UploadRegistryEntry,
	generation: number,
) => {
	if (!entry.fileId || !isCurrentGeneration(entry, generation)) {
		return;
	}
	const result = upsertChatDraftAttachmentRecord({
		status: "uploaded",
		clientId: entry.clientId,
		fileId: entry.fileId,
		fileName: entry.file.name,
		fileType: entry.file.type,
		lastModified: entry.file.lastModified,
		size: entry.file.size,
		organizationId: entry.organizationId,
		chatId: entry.chatId,
	});
	if (result.ok) {
		entry.draftWarning = undefined;
		return;
	}
	entry.draftWarning = uploadedDraftWarning;
};

const beginUpload = (entry: UploadRegistryEntry) => {
	if (entry.uploadStarted) {
		return;
	}
	entry.uploadStarted = true;
	const generation = entry.generation;
	entry.status = "uploading";
	notifySubscribers(entry);
	void persistUploadPayload(entry, generation);
	void (async () => {
		try {
			const result = await API.experimental.uploadChatFile(
				entry.file,
				entry.organizationId,
			);
			if (!isCurrentGeneration(entry, generation)) {
				return;
			}
			entry.status = "uploaded";
			entry.fileId = result.id;
			entry.error = undefined;
			persistUploadedRecord(entry, generation);
			if (entry.file.type.startsWith("image/")) {
				void fetch(getChatFileURL(result.id)).catch(() => undefined);
			}
			notifySubscribers(entry);
			pruneTerminalRegistryEntry(entry);
		} catch (error) {
			if (!isCurrentGeneration(entry, generation)) {
				return;
			}
			entry.status = "error";
			entry.error = formatAgentAttachmentUploadError(error);
			notifySubscribers(entry);
			pruneTerminalRegistryEntry(entry);
		}
	})();
};

const removeRegistryEntry = (clientId: string) => {
	const entry = activeDraftUploads.get(clientId);
	if (!entry) {
		return;
	}
	entry.generation += 1;
	entry.removed = true;
	activeDraftUploads.delete(clientId);
	notifySubscribers(entry);
};

const viewsFromRestored = (
	restored: readonly RestoredChatDraftAttachment[],
): DraftAttachmentView[] =>
	restored.map(({ record, file }) => {
		const status = record.status === "uploaded" ? "uploaded" : record.status;
		const fileId = record.status === "uploaded" ? record.fileId : undefined;
		return {
			clientId: record.clientId,
			file,
			fileId,
			status,
			...computePreview(file, status, fileId),
		};
	});

const viewFromSnapshot = (
	snapshot: UploadRegistrySnapshot,
): DraftAttachmentView => ({
	clientId: snapshot.clientId,
	file: snapshot.file,
	fileId: snapshot.fileId,
	status: snapshot.status,
	error: snapshot.error,
	draftWarning: snapshot.draftWarning,
	...computePreview(snapshot.file, snapshot.status, snapshot.fileId),
});

const applySnapshot = (
	views: readonly DraftAttachmentView[],
	snapshot: UploadRegistrySnapshot,
): DraftAttachmentView[] => {
	if (snapshot.removed) {
		return views.filter((view) => {
			if (view.clientId !== snapshot.clientId) {
				return true;
			}
			revokeBlobPreview(view);
			return false;
		});
	}
	let found = false;
	const next = views.map((view) => {
		if (view.clientId !== snapshot.clientId) {
			return view;
		}
		found = true;
		const nextPreview = computePreview(
			snapshot.file,
			snapshot.status,
			snapshot.fileId,
			{ previewUrl: view.previewUrl, previewUrlKind: view.previewUrlKind },
		);
		if (view.previewUrl !== nextPreview.previewUrl) {
			revokeBlobPreview(view);
		}
		return {
			...view,
			file: snapshot.file,
			fileId: snapshot.fileId,
			status: snapshot.status,
			error: snapshot.error,
			draftWarning: snapshot.draftWarning,
			previewUrl: nextPreview.previewUrl,
			previewUrlKind: nextPreview.previewUrlKind,
		};
	});
	if (found) {
		return next;
	}
	return [...next, viewFromSnapshot(snapshot)];
};

const isSameScope = (
	entry: UploadRegistryEntry,
	organizationId: string,
	chatId: string,
) =>
	!entry.removed &&
	entry.organizationId === organizationId &&
	entry.chatId === chatId;

const getDraftScopeKey = (
	organizationId: string | undefined,
	chatId: string | undefined,
) => (organizationId && chatId ? `${organizationId}:${chatId}` : undefined);

const removeRegistryEntriesForScope = (
	organizationId: string,
	chatId: string,
) => {
	for (const entry of Array.from(activeDraftUploads.values())) {
		if (entry.organizationId === organizationId && entry.chatId === chatId) {
			removeRegistryEntry(entry.clientId);
		}
	}
};

const hydrateViews = (
	organizationId: string | undefined,
	chatId: string | undefined,
) => {
	if (!organizationId || !chatId) {
		return [];
	}
	let views = viewsFromRestored(
		restoreChatDraftAttachments(organizationId, chatId),
	);
	for (const entry of activeDraftUploads.values()) {
		if (!isSameScope(entry, organizationId, chatId)) {
			continue;
		}
		views = applySnapshot(views, snapshotFromEntry(entry));
	}
	return views;
};

const unsubscribeAllEntries = (subscriptions: {
	current: Map<string, () => void>;
}) => {
	for (const unsubscribe of subscriptions.current.values()) {
		unsubscribe();
	}
	subscriptions.current.clear();
};

const subscribeToEntry = (
	entry: UploadRegistryEntry,
	subscriptions: { current: Map<string, () => void> },
	subscriber: UploadRegistrySubscriber,
) => {
	if (entry.removed || subscriptions.current.has(entry.clientId)) {
		return;
	}
	entry.subscribers.add(subscriber);
	subscriptions.current.set(entry.clientId, () => {
		entry.subscribers.delete(subscriber);
		pruneTerminalRegistryEntry(entry);
	});
};

type SetDraftAttachmentViews = (
	updater: (prev: DraftAttachmentView[]) => DraftAttachmentView[],
) => void;

const queueTextContentReads = (
	candidateViews: readonly DraftAttachmentView[],
	setDraftViews: SetDraftAttachmentViews,
	shouldApplyResult: () => boolean,
) => {
	for (const view of candidateViews) {
		if (
			view.status === "error" ||
			view.status === "uploaded" ||
			view.textContent !== undefined ||
			view.file.type !== "text/plain" ||
			view.file.size > maxTextPreviewSize
		) {
			continue;
		}
		void readAgentAttachmentText(view.file)
			.then((content) => {
				if (!shouldApplyResult()) {
					return;
				}
				setDraftViews((prev) => {
					let updated = false;
					const next = prev.map((current) => {
						if (current.clientId !== view.clientId) {
							return current;
						}
						updated = true;
						return { ...current, textContent: content };
					});
					return updated ? next : prev;
				});
			})
			.catch((error) => {
				console.error("Failed to read text file content:", error);
			});
	}
};

export function useChatDraftAttachments(
	organizationId: string | undefined,
	chatId: string | undefined,
	options?: { provider?: string },
) {
	const [views, setViews] = useState(() =>
		hydrateViews(organizationId, chatId),
	);
	const viewsRef = useRef(views);
	const subscriptionsRef = useRef(new Map<string, () => void>());
	const scopeRef = useRef(getDraftScopeKey(organizationId, chatId));
	// providerRef lets event-driven handlers (paste/drop) see the
	// latest model selection without rebuilding handleAttach. The
	// effect-based write keeps React Compiler happy.
	const provider = options?.provider;
	const providerRef = useRef(provider);
	useEffect(() => {
		providerRef.current = provider;
	}, [provider]);
	// clientIds whose resize is in flight but the user removed the
	// attachment (or the chat scope changed). processResize checks
	// this before swapping in a replacement.
	const abandonedResizesRef = useRef<Set<string>>(new Set());
	const [subscriber] = useState<UploadRegistrySubscriber>(
		() =>
			function handleUploadRegistrySnapshot(snapshot: UploadRegistrySnapshot) {
				setViews((prev) => applySnapshot(prev, snapshot));
			},
	);

	useEffect(() => {
		viewsRef.current = views;
	}, [views]);

	useEffect(() => {
		return () => {
			scopeRef.current = undefined;
			unsubscribeAllEntries(subscriptionsRef);
			for (const view of viewsRef.current) {
				revokeBlobPreview(view);
			}
		};
	}, []);

	useEffect(() => {
		const scopeKey = getDraftScopeKey(organizationId, chatId);
		scopeRef.current = scopeKey;
		unsubscribeAllEntries(subscriptionsRef);
		// Abandon in-flight resizes from the previous scope so
		// their callbacks don't register uploads in the new scope.
		for (const view of viewsRef.current) {
			abandonedResizesRef.current.add(view.clientId);
		}
		if (!organizationId || !chatId || !scopeKey) {
			setViews([]);
			return;
		}
		const previousViews = viewsRef.current;
		const restored = restoreChatDraftAttachments(organizationId, chatId);
		let nextViews = viewsFromRestored(restored);
		for (const entry of activeDraftUploads.values()) {
			if (!isSameScope(entry, organizationId, chatId)) {
				continue;
			}
			subscribeToEntry(entry, subscriptionsRef, subscriber);
			nextViews = applySnapshot(nextViews, snapshotFromEntry(entry));
		}
		const restoredEntriesToStart: UploadRegistryEntry[] = [];
		for (const { record, file } of restored) {
			if (record.status === "uploaded") {
				continue;
			}
			const entry = createRegistryEntry(
				record.clientId,
				organizationId,
				chatId,
				file,
			);
			subscribeToEntry(entry, subscriptionsRef, subscriber);
			restoredEntriesToStart.push(entry);
		}
		for (const view of previousViews) {
			revokeBlobPreview(view);
		}
		setViews(nextViews);
		queueTextContentReads(
			nextViews,
			setViews,
			() => scopeRef.current === scopeKey,
		);
		for (const entry of restoredEntriesToStart) {
			beginUpload(entry);
		}
	}, [organizationId, chatId, subscriber]);

	// The view enters in "processing" status from handleAttach;
	// processResize either swaps in the smaller file and registers
	// the upload, or surfaces a too-large error.
	const processResize = async (
		clientId: string,
		original: File,
		budget: number,
		// Pinned at attach time so a mid-resize provider switch
		// can't mislabel the error with the new provider's name.
		providerSnapshot: string | undefined,
	) => {
		let resized: File | null = null;
		try {
			resized = await resizeImageToMaxBytes(original, budget);
		} catch {
			resized = null;
		}

		if (abandonedResizesRef.current.has(clientId)) {
			return;
		}
		if (!organizationId || !chatId) {
			return;
		}
		const scopeKey = getDraftScopeKey(organizationId, chatId);
		if (!scopeKey || scopeRef.current !== scopeKey) {
			return;
		}

		const replacement = resized ?? original;
		const replaced = replacement !== original;

		// Resize failed entirely or couldn't shrink enough; show
		// the too-large error instead of uploading and 413-ing.
		if (replacement.size > MaxChatFileSizeBytes) {
			setViews((prev) =>
				prev.map((view) => {
					if (view.clientId !== clientId) {
						return view;
					}
					if (replaced) {
						revokeBlobPreview(view);
					}
					const previewState = replaced
						? computePreview(replacement, "error")
						: {
								previewUrl: view.previewUrl,
								previewUrlKind: view.previewUrlKind,
							};
					return {
						...view,
						file: replacement,
						status: "error",
						error: formatAgentAttachmentTooLargeError(replacement.size),
						previewUrl: previewState.previewUrl,
						previewUrlKind: previewState.previewUrlKind,
					};
				}),
			);
			return;
		}

		// Replacement is still over the provider budget (e.g.
		// animated GIF on Anthropic that we don't re-encode).
		// Surface the error at attach time rather than letting
		// the server backstop reject only at send time.
		if (replacement.type.startsWith("image/") && replacement.size > budget) {
			setViews((prev) =>
				prev.map((view) => {
					if (view.clientId !== clientId) {
						return view;
					}
					if (replaced) {
						revokeBlobPreview(view);
					}
					const previewState = replaced
						? computePreview(replacement, "error")
						: {
								previewUrl: view.previewUrl,
								previewUrlKind: view.previewUrlKind,
							};
					return {
						...view,
						file: replacement,
						status: "error",
						error: providerBudgetError(
							providerSnapshot,
							replacement.size,
							budget,
						),
						previewUrl: previewState.previewUrl,
						previewUrlKind: previewState.previewUrlKind,
					};
				}),
			);
			return;
		}

		// beginUpload below drives the view from pending to uploading
		// via subscribers; we set "pending" here so the registry's
		// initial snapshot doesn't overwrite our blob preview.
		setViews((prev) =>
			prev.map((view) => {
				if (view.clientId !== clientId) {
					return view;
				}
				if (!replaced) {
					return { ...view, status: "pending" };
				}
				revokeBlobPreview(view);
				const nextPreview = computePreview(replacement, "pending");
				return {
					...view,
					file: replacement,
					status: "pending",
					previewUrl: nextPreview.previewUrl,
					previewUrlKind: nextPreview.previewUrlKind,
				};
			}),
		);

		const entry = createRegistryEntry(
			clientId,
			organizationId,
			chatId,
			replacement,
		);
		subscribeToEntry(entry, subscriptionsRef, subscriber);
		beginUpload(entry);
	};

	const handleAttach = (incomingFiles: File[]) => {
		// Sanitize filenames at the boundary so chip labels, the
		// chat-draft localStorage record, the upload header, and any
		// downstream LLM prompt all see safe names. Already-safe
		// names return the same File by reference.
		const files = incomingFiles.map(renameChatFileForUpload);
		const scopeKey = getDraftScopeKey(organizationId, chatId);
		// Snapshot provider + budget so a mid-resize switch
		// can't relabel the error with the new provider.
		const providerSnapshot = providerRef.current;
		const budget = imageBudgetForProvider(providerSnapshot);
		const entriesToStart: UploadRegistryEntry[] = [];
		const resizeJobs: Array<{ clientId: string; file: File }> = [];
		const nextViews: DraftAttachmentView[] = [];
		for (const file of files) {
			const clientId = createClientId();
			const baseView: DraftAttachmentView = {
				clientId,
				file,
				status: "pending",
			};
			const needsResize = imageNeedsResize(file, budget);
			// Non-image files over the upload cap are rejected.
			// Oversized images take the resize pipeline regardless;
			// the post-resize check validates the result.
			if (file.size > MaxChatFileSizeBytes && !needsResize) {
				nextViews.push({
					...baseView,
					status: "error",
					error: formatAgentAttachmentTooLargeError(file.size),
				});
				continue;
			}
			if (!organizationId || !chatId || !scopeKey) {
				nextViews.push({
					...baseView,
					status: "error",
					error: "Unable to upload: no chat context.",
				});
				continue;
			}
			if (needsResize) {
				// Commit synchronously with "processing" so the
				// send gate blocks dispatch until resize finishes.
				const view = {
					...baseView,
					status: "processing" as const,
					...computePreview(file, "processing"),
				};
				nextViews.push(view);
				resizeJobs.push({ clientId, file });
				continue;
			}
			const view = { ...baseView, ...computePreview(file, "pending") };
			const entry = createRegistryEntry(clientId, organizationId, chatId, file);
			subscribeToEntry(entry, subscriptionsRef, subscriber);
			nextViews.push(view);
			entriesToStart.push(entry);
		}
		setViews((prev) => [...prev, ...nextViews]);
		for (const entry of entriesToStart) {
			beginUpload(entry);
		}
		queueTextContentReads(
			nextViews,
			setViews,
			() => scopeRef.current === scopeKey,
		);
		// processResize re-checks abandonment + scope before
		// mutating state, so it's safe to fire-and-forget.
		for (const job of resizeJobs) {
			void processResize(job.clientId, job.file, budget, providerSnapshot);
		}
	};

	const handleRemoveAttachment = (attachment: number | File) => {
		const index =
			typeof attachment === "number"
				? attachment
				: views.findIndex((view) => view.file === attachment);
		const removed = index >= 0 ? views[index] : undefined;
		if (!removed) {
			return;
		}
		// In-flight resize would otherwise swap in a replacement
		// after the clear below.
		abandonedResizesRef.current.add(removed.clientId);
		if (organizationId && chatId) {
			removeChatDraftAttachmentRecord(organizationId, chatId, removed.clientId);
		}
		removeRegistryEntry(removed.clientId);
		setViews((prev) =>
			prev.filter((view) => {
				if (view.clientId !== removed.clientId) {
					return true;
				}
				revokeBlobPreview(view);
				return false;
			}),
		);
	};

	const resetAttachments = () => {
		// Abandon all in-flight resizes so they don't swap a
		// replacement back in (which would also re-call beginUpload
		// against the now-stale scope).
		for (const view of viewsRef.current) {
			abandonedResizesRef.current.add(view.clientId);
		}
		if (!organizationId || !chatId) {
			setViews([]);
			return;
		}
		clearChatDraftAttachmentRecords(organizationId, chatId);
		const resetScopeKey = getDraftScopeKey(organizationId, chatId);
		if (scopeRef.current !== resetScopeKey) {
			removeRegistryEntriesForScope(organizationId, chatId);
			return;
		}
		for (const view of viewsRef.current) {
			revokeBlobPreview(view);
			removeRegistryEntry(view.clientId);
		}
		setViews([]);
	};

	// React Compiler memoizes pure derived values in this directory.
	// Keep these inline rather than adding manual memoization.
	const attachments = views.map((view) => view.file);
	const uploadStates = new Map<File, UploadState>();
	const previewUrls = new Map<File, string>();
	const textContents = new Map<File, string>();
	for (const view of views) {
		uploadStates.set(view.file, {
			status: view.status,
			fileId: view.fileId,
			error: view.error,
			draftWarning: view.draftWarning,
		});
		if (view.previewUrl) {
			previewUrls.set(view.file, view.previewUrl);
		}
		if (view.textContent !== undefined) {
			textContents.set(view.file, view.textContent);
		}
	}

	return {
		attachments,
		textContents,
		uploadStates,
		previewUrls,
		handleAttach,
		handleRemoveAttachment,
		resetAttachments,
	};
}

/** @internal Exported for tests. */
export const resetChatDraftAttachmentRegistryForTest = () => {
	for (const entry of activeDraftUploads.values()) {
		entry.generation += 1;
		entry.removed = true;
		notifySubscribers(entry);
	}
	activeDraftUploads.clear();
	fallbackClientIdCounter = 0;
};
