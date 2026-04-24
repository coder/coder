import { useEffect, useRef, useState } from "react";
import { API } from "#/api/api";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import type { UploadState } from "../components/AgentChatInput";
import { getChatFileURL } from "../utils/chatAttachments";
import {
	clearChatDraftAttachmentRecords,
	fileToDataURL,
	type RestoredChatDraftAttachment,
	removeChatDraftAttachmentRecord,
	restoreChatDraftAttachments,
	upsertChatDraftAttachmentRecord,
} from "../utils/chatDraftAttachmentStorage";

const maxAttachmentSize = 10 * 1024 * 1024;

const pendingDraftWarning =
	"This file is attached for now, but it is too large to save as a draft. If you leave this chat before it uploads or sends, it may be lost.";
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
	memoryOnly?: boolean;
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
	memoryOnly?: boolean;
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
	memoryOnly?: boolean;
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
	memoryOnly: entry.memoryOnly,
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

const formatUploadError = (error: unknown) => {
	const message = getErrorMessage(error, "Upload failed");
	const detail = getErrorDetail(error);
	return detail ? `${message} ${detail}` : message;
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
		entry.memoryOnly = true;
		entry.draftWarning = pendingDraftWarning;
		notifySubscribers(entry);
	} catch {
		if (!isCurrentGeneration(entry, generation)) {
			return;
		}
		entry.memoryOnly = true;
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
		entry.memoryOnly = false;
		entry.draftWarning = undefined;
		return;
	}
	entry.memoryOnly = true;
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
		} catch (error) {
			if (!isCurrentGeneration(entry, generation)) {
				return;
			}
			entry.status = "error";
			entry.error = formatUploadError(error);
			notifySubscribers(entry);
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
	restored.map(({ record, file }) => ({
		clientId: record.clientId,
		file,
		fileId: record.status === "uploaded" ? record.fileId : undefined,
		status: record.status === "uploaded" ? "uploaded" : record.status,
		previewUrl:
			record.status === "uploaded" && file.type.startsWith("image/")
				? getChatFileURL(record.fileId)
				: createBlobPreview(file),
		previewUrlKind:
			record.status === "uploaded" && file.type.startsWith("image/")
				? "chatFile"
				: file.type !== "text/plain"
					? "blob"
					: undefined,
	}));

const viewFromSnapshot = (
	snapshot: UploadRegistrySnapshot,
): DraftAttachmentView => ({
	clientId: snapshot.clientId,
	file: snapshot.file,
	fileId: snapshot.fileId,
	status: snapshot.status,
	error: snapshot.error,
	draftWarning: snapshot.draftWarning,
	memoryOnly: snapshot.memoryOnly,
	previewUrl:
		snapshot.status === "uploaded" && snapshot.fileId
			? getChatFileURL(snapshot.fileId)
			: createBlobPreview(snapshot.file),
	previewUrlKind:
		snapshot.status === "uploaded" && snapshot.fileId
			? "chatFile"
			: snapshot.file.type !== "text/plain"
				? "blob"
				: undefined,
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
		const nextPreviewUrl =
			snapshot.status === "uploaded" && snapshot.fileId
				? getChatFileURL(snapshot.fileId)
				: view.previewUrl;
		const nextPreviewUrlKind =
			snapshot.status === "uploaded" && snapshot.fileId
				? "chatFile"
				: view.previewUrlKind;
		if (view.previewUrl !== nextPreviewUrl) {
			revokeBlobPreview(view);
		}
		return {
			...view,
			file: snapshot.file,
			fileId: snapshot.fileId,
			status: snapshot.status,
			error: snapshot.error,
			draftWarning: snapshot.draftWarning,
			memoryOnly: snapshot.memoryOnly,
			previewUrl: nextPreviewUrl,
			previewUrlKind: nextPreviewUrlKind,
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
	});
};

const readTextContent = async (file: File): Promise<string> => {
	if (typeof file.text === "function") {
		return file.text();
	}
	return new Response(file).text();
};

export function useChatDraftAttachments(
	organizationId: string | undefined,
	chatId: string | undefined,
) {
	const [views, setViews] = useState(() =>
		hydrateViews(organizationId, chatId),
	);
	const viewsRef = useRef(views);
	const subscriptionsRef = useRef(new Map<string, () => void>());
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
			unsubscribeAllEntries(subscriptionsRef);
			for (const view of viewsRef.current) {
				revokeBlobPreview(view);
			}
		};
	}, []);

	useEffect(() => {
		unsubscribeAllEntries(subscriptionsRef);
		if (!organizationId || !chatId) {
			setViews([]);
			return;
		}
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
		setViews(nextViews);
		for (const entry of restoredEntriesToStart) {
			beginUpload(entry);
		}
	}, [organizationId, chatId, subscriber]);

	const handleAttach = (files: File[]) => {
		const entriesToStart: UploadRegistryEntry[] = [];
		const nextViews: DraftAttachmentView[] = [];
		for (const file of files) {
			const clientId = createClientId();
			const previewUrl = createBlobPreview(file);
			const baseView: DraftAttachmentView = {
				clientId,
				file,
				status: "pending",
				previewUrl,
				previewUrlKind: previewUrl ? "blob" : undefined,
			};
			if (file.size > maxAttachmentSize) {
				nextViews.push({
					...baseView,
					status: "error",
					error: `File too large (${(file.size / 1024 / 1024).toFixed(1)} MB). Maximum is 10 MB.`,
				});
				continue;
			}
			if (!organizationId || !chatId) {
				nextViews.push({
					...baseView,
					status: "error",
					error: "Unable to upload: no chat context.",
				});
				continue;
			}
			const entry = createRegistryEntry(clientId, organizationId, chatId, file);
			subscribeToEntry(entry, subscriptionsRef, subscriber);
			nextViews.push(baseView);
			entriesToStart.push(entry);
		}
		setViews((prev) => [...prev, ...nextViews]);
		for (const entry of entriesToStart) {
			beginUpload(entry);
		}
		for (const view of nextViews) {
			if (
				view.file.type !== "text/plain" ||
				view.file.size > maxAttachmentSize
			) {
				continue;
			}
			void readTextContent(view.file)
				.then((content) => {
					setViews((prev) =>
						prev.map((current) =>
							current.clientId === view.clientId
								? { ...current, textContent: content }
								: current,
						),
					);
				})
				.catch((error) => {
					console.error("Failed to read text file content:", error);
				});
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
		for (const view of views) {
			revokeBlobPreview(view);
			removeRegistryEntry(view.clientId);
		}
		if (organizationId && chatId) {
			clearChatDraftAttachmentRecords(organizationId, chatId);
		}
		setViews([]);
	};

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
