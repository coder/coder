import {
	type Dispatch,
	type SetStateAction,
	useEffect,
	useEffectEvent,
	useRef,
	useState,
} from "react";
import { API } from "#/api/api";
import { MaxChatFileSizeBytes } from "#/api/typesGenerated";
import type { UploadState } from "../components/AgentChatInput";
import {
	getChatFileURL,
	renameChatFileForUpload,
} from "../utils/chatAttachments";
import {
	formatAgentAttachmentTooLargeError,
	formatAgentAttachmentUploadError,
} from "../utils/fileAttachmentLimits";
import {
	imageBudgetForProvider,
	imageNeedsResize,
	providerBudgetError,
} from "../utils/imageBudget";
import { resizeImageToMaxBytes } from "../utils/resizeImage";

/** @internal Exported for testing. */
export const persistedAttachmentsStorageKey = "agents.persisted-attachments";

/**
 * Serializable metadata stored in localStorage so that already-uploaded
 * attachments survive page navigations on the create form.
 */
interface PersistedAttachment {
	fileId: string;
	fileName: string;
	fileType: string;
	lastModified: number;
	organizationId: string;
}

/**
 * Restore previously persisted attachments from localStorage.
 * Creates synthetic File objects (empty blobs with correct metadata)
 * and populates the corresponding Maps so the UI can render them.
 *
 * Only attachments matching `currentOrgId` are returned. Entries
 * belonging to a different organization are pruned from storage.
 */
function restorePersistedAttachments(currentOrgId: string): {
	attachments: File[];
	uploadStates: Map<File, UploadState>;
	previewUrls: Map<File, string>;
} {
	// Skip when org ID isn't loaded yet so we don't prune valid
	// entries. The initializer runs once, so callers must wait for
	// the org ID before mounting.
	if (!currentOrgId) {
		return {
			attachments: [],
			uploadStates: new Map(),
			previewUrls: new Map(),
		};
	}
	const stored = localStorage.getItem(persistedAttachmentsStorageKey);
	if (!stored) {
		return {
			attachments: [],
			uploadStates: new Map(),
			previewUrls: new Map(),
		};
	}
	try {
		const persisted: PersistedAttachment[] = JSON.parse(stored);
		const matched = persisted.filter((p) => p.organizationId === currentOrgId);

		if (matched.length !== persisted.length) {
			if (matched.length > 0) {
				localStorage.setItem(
					persistedAttachmentsStorageKey,
					JSON.stringify(matched),
				);
			} else {
				localStorage.removeItem(persistedAttachmentsStorageKey);
			}
		}

		const attachments: File[] = [];
		const uploadStates = new Map<File, UploadState>();
		const previewUrls = new Map<File, string>();

		for (const p of matched) {
			if (!p.fileId || !p.fileName) continue;
			// Synthetic File used as a Map key only; the existing
			// file_id is reused at send time.
			const file = new File([], p.fileName, {
				type: p.fileType,
				lastModified: p.lastModified,
			});
			attachments.push(file);
			uploadStates.set(file, { status: "uploaded", fileId: p.fileId });
			if (p.fileType.startsWith("image/")) {
				previewUrls.set(file, getChatFileURL(p.fileId));
			}
		}
		return { attachments, uploadStates, previewUrls };
	} catch {
		return {
			attachments: [],
			uploadStates: new Map(),
			previewUrls: new Map(),
		};
	}
}

function addPersistedAttachment(
	file: File,
	fileId: string,
	organizationId: string,
) {
	const stored = localStorage.getItem(persistedAttachmentsStorageKey);
	let persisted: PersistedAttachment[];
	try {
		persisted = stored ? JSON.parse(stored) : [];
	} catch {
		persisted = [];
	}
	persisted.push({
		fileId,
		fileName: file.name,
		fileType: file.type,
		lastModified: file.lastModified,
		organizationId,
	});
	localStorage.setItem(
		persistedAttachmentsStorageKey,
		JSON.stringify(persisted),
	);
}

function removePersistedAttachment(fileId: string) {
	const stored = localStorage.getItem(persistedAttachmentsStorageKey);
	if (!stored) {
		return;
	}
	try {
		const persisted: PersistedAttachment[] = JSON.parse(stored);
		const filtered = persisted.filter((p) => p.fileId !== fileId);
		if (filtered.length > 0) {
			localStorage.setItem(
				persistedAttachmentsStorageKey,
				JSON.stringify(filtered),
			);
		} else {
			localStorage.removeItem(persistedAttachmentsStorageKey);
		}
	} catch {
		localStorage.removeItem(persistedAttachmentsStorageKey);
	}
}

function clearPersistedAttachments() {
	localStorage.removeItem(persistedAttachmentsStorageKey);
}

interface UseFileAttachmentsReturn {
	attachments: File[];
	textContents: Map<File, string>;
	uploadStates: Map<File, UploadState>;
	previewUrls: Map<File, string>;
	handleAttach: (files: File[]) => void;
	handleRemoveAttachment: (attachment: number | File) => void;
	startUpload: (file: File) => void;
	resetAttachments: () => void;
	setAttachments: Dispatch<SetStateAction<File[]>>;
	setPreviewUrls: Dispatch<SetStateAction<Map<File, string>>>;
	setUploadStates: Dispatch<SetStateAction<Map<File, UploadState>>>;
}

export function useFileAttachments(
	organizationId: string | undefined,
	options?: { persist?: boolean; provider?: string },
): UseFileAttachmentsReturn {
	const persist = options?.persist ?? false;

	// providerRef lets event-driven handlers (paste/drop) see the
	// latest model selection without rebuilding handleAttach. The
	// effect-based write keeps React Compiler happy.
	const provider = options?.provider;
	const providerRef = useRef(provider);
	useEffect(() => {
		providerRef.current = provider;
	}, [provider]);

	const [restored] = useState(() =>
		persist
			? restorePersistedAttachments(organizationId ?? "")
			: {
					attachments: [] as File[],
					uploadStates: new Map<File, UploadState>(),
					previewUrls: new Map<File, string>(),
				},
	);

	const [attachments, setAttachments] = useState<File[]>(restored.attachments);
	const [uploadStates, setUploadStates] = useState(restored.uploadStates);
	const [previewUrls, setPreviewUrls] = useState(restored.previewUrls);
	const [textContents, setTextContents] = useState(
		() => new Map<File, string>(),
	);

	const revokePreviewUrls = useEffectEvent(() => {
		for (const [, url] of previewUrls) {
			if (url.startsWith("blob:")) URL.revokeObjectURL(url);
		}
	});

	// Revoke blob URLs on unmount to prevent memory leaks.
	useEffect(() => {
		return () => revokePreviewUrls();
	}, []);

	const startUpload = (file: File) => {
		if (!organizationId) {
			setUploadStates((prev) =>
				new Map(prev).set(file, {
					status: "error",
					error: "Unable to upload: no organization context.",
				}),
			);
			return;
		}

		const shouldPersist = persist && Boolean(organizationId);
		const isImage = file.type.startsWith("image/");

		setUploadStates((prev) => new Map(prev).set(file, { status: "uploading" }));
		void (async () => {
			try {
				const result = await API.experimental.uploadChatFile(
					file,
					organizationId,
				);
				setUploadStates((prev) =>
					new Map(prev).set(file, {
						status: "uploaded",
						fileId: result.id,
					}),
				);
				if (shouldPersist) {
					addPersistedAttachment(file, result.id, organizationId!);
				}
				if (isImage) {
					// Pre-warm the HTTP cache so the timeline can
					// render the image instantly after send. Text
					// content is already local in the composer.
					void fetch(getChatFileURL(result.id));
				}
			} catch (err: unknown) {
				const errorMessage = formatAgentAttachmentUploadError(err);
				setUploadStates((prev) =>
					new Map(prev).set(file, {
						status: "error",
						error: errorMessage,
					}),
				);
			}
		})();
	};

	// Files removed while their resize is in flight. processResizes
	// checks this before swapping in a replacement so a dismissed
	// file can't be resurrected. WeakSet lets entries get GC'd.
	const abandonedResizesRef = useRef<WeakSet<File>>(new WeakSet());

	type AttachItem = { file: File; needsResize: boolean };

	const processResizes = async (
		items: readonly AttachItem[],
		budget: number,
		// Pinned at attach time so a mid-resize provider switch
		// can't mislabel the error with the new provider's name.
		providerSnapshot: string | undefined,
	) => {
		// Sequential so each swap commits before the next starts;
		// resizeImageToMaxBytes already serializes decode work.
		for (const { file: original, needsResize } of items) {
			if (!needsResize) continue;
			let resized: File | null = null;
			try {
				resized = await resizeImageToMaxBytes(original, budget);
			} catch {
				resized = null;
			}

			// Skip if the user removed this attachment while
			// resizing; updates here would resurrect it.
			if (abandonedResizesRef.current.has(original)) {
				continue;
			}
			const replacement = resized ?? original;
			const replaced = replacement !== original;

			// Functional updaters: if a racing removal cleared the
			// original, every updater below becomes a no-op.
			setAttachments((prev) => {
				const idx = prev.indexOf(original);
				if (idx === -1 || !replaced) return prev;
				const next = prev.slice();
				next[idx] = replacement;
				return next;
			});
			setPreviewUrls((prev) => {
				// Skip when no replacement happened so we don't
				// revoke the original's still-in-use blob URL.
				if (!prev.has(original) || !replaced) return prev;
				const next = new Map(prev);
				const oldUrl = next.get(original);
				if (oldUrl?.startsWith("blob:")) URL.revokeObjectURL(oldUrl);
				next.delete(original);
				if (replacement.type !== "text/plain") {
					next.set(replacement, URL.createObjectURL(replacement));
				}
				return next;
			});
			setUploadStates((prev) => {
				// Skip when no replacement: startUpload below
				// overwrites "processing" with "uploading".
				if (!prev.has(original) || !replaced) return prev;
				const next = new Map(prev);
				next.delete(original);
				return next;
			});

			// Resize failed and the original still exceeds the
			// server cap; show the too-large error instead of
			// kicking off an upload that will 413.
			if (replacement.size > MaxChatFileSizeBytes) {
				setUploadStates((prev) =>
					new Map(prev).set(replacement, {
						status: "error" as const,
						error: formatAgentAttachmentTooLargeError(replacement.size),
					}),
				);
				continue;
			}
			// Replacement is still over the provider budget (e.g.
			// animated GIF on Anthropic that we don't re-encode).
			// Surface the error at attach time rather than letting
			// the server backstop reject only at send time.
			if (replacement.type.startsWith("image/") && replacement.size > budget) {
				setUploadStates((prev) =>
					new Map(prev).set(replacement, {
						status: "error" as const,
						error: providerBudgetError(
							providerSnapshot,
							replacement.size,
							budget,
						),
					}),
				);
				continue;
			}
			startUpload(replacement);
		}
	};

	const handleAttach = (incomingFiles: File[]) => {
		// Sanitize filenames at the boundary so chip labels, the
		// persisted-attachment localStorage record, the upload
		// header, and any downstream LLM prompt all see safe names.
		// Already-safe names return the same File by reference; the
		// File identity is used as a Map key below (previewUrls,
		// uploadStates, textContents).
		const files = incomingFiles.map(renameChatFileForUpload);
		// Originals enter state with a "processing" status so the
		// send gate blocks dispatch until processResizes finishes.
		// Snapshot provider + budget so a mid-resize switch can't
		// relabel the error with the new provider.
		const providerSnapshot = providerRef.current;
		const budget = imageBudgetForProvider(providerSnapshot);
		const items: AttachItem[] = files.map((file) => ({
			file,
			needsResize: imageNeedsResize(file, budget),
		}));

		setAttachments((prev) => [...prev, ...files]);
		setPreviewUrls((prev) => {
			const next = new Map(prev);
			for (const { file } of items) {
				if (file.type !== "text/plain") {
					next.set(file, URL.createObjectURL(file));
				}
			}
			return next;
		});
		setUploadStates((prev) => {
			const next = new Map(prev);
			for (const { file, needsResize } of items) {
				if (file.size > MaxChatFileSizeBytes && !needsResize) {
					next.set(file, {
						status: "error" as const,
						error: formatAgentAttachmentTooLargeError(file.size),
					});
				} else if (needsResize) {
					next.set(file, { status: "processing" });
				}
			}
			return next;
		});

		for (const { file } of items) {
			if (file.type === "text/plain" && file.size <= MaxChatFileSizeBytes) {
				// Some test environments lack File.prototype.text.
				const readText =
					typeof file.text === "function"
						? file.text()
						: new Response(file).text();
				void readText
					.then((content) => {
						setTextContents((prev) => {
							const next = new Map(prev);
							next.set(file, content);
							return next;
						});
					})
					.catch((err) => {
						console.error("Failed to read text file content:", err);
					});
			}
		}

		for (const { file, needsResize } of items) {
			if (needsResize) continue;
			if (file.size > MaxChatFileSizeBytes) continue; // already marked as error above
			startUpload(file);
		}

		void processResizes(items, budget, providerSnapshot);
	};

	const handleRemoveAttachment = (attachment: number | File) => {
		// Side effects (localStorage, abandonment) happen here;
		// React may call updaters multiple times under StrictMode
		// or React Compiler, so they must stay pure.
		const idx =
			typeof attachment === "number"
				? attachment
				: attachments.indexOf(attachment);
		const removed = idx >= 0 ? attachments[idx] : undefined;
		if (removed) {
			// In-flight resize would otherwise resurrect this file
			// by swapping in a replacement after the clear below.
			abandonedResizesRef.current.add(removed);
		}
		if (persist && removed) {
			const state = uploadStates.get(removed);
			if (state?.status === "uploaded" && state.fileId) {
				removePersistedAttachment(state.fileId);
			}
		}

		setAttachments((prev) => {
			const index =
				typeof attachment === "number" ? attachment : prev.indexOf(attachment);
			if (index === -1) {
				return prev;
			}

			const removedFile = prev[index];
			setUploadStates((prevStates) => {
				const next = new Map(prevStates);
				next.delete(removedFile);
				return next;
			});
			setPreviewUrls((prevUrls) => {
				const url = prevUrls.get(removedFile);
				if (url?.startsWith("blob:")) URL.revokeObjectURL(url);
				const next = new Map(prevUrls);
				next.delete(removedFile);
				return next;
			});
			setTextContents((prevContents) => {
				const next = new Map(prevContents);
				next.delete(removedFile);
				return next;
			});
			return prev.filter((_, i) => i !== index);
		});
	};

	const resetAttachments = () => {
		// Abandon all in-flight resizes so they don't swap a
		// replacement back in (which would also re-call startUpload
		// against the now-stale scope).
		for (const file of attachments) {
			abandonedResizesRef.current.add(file);
		}
		for (const [, url] of previewUrls) {
			if (url.startsWith("blob:")) URL.revokeObjectURL(url);
		}
		setPreviewUrls(new Map());
		setTextContents(new Map());
		setUploadStates(new Map());
		setAttachments([]);
		if (persist) {
			clearPersistedAttachments();
		}
	};

	return {
		attachments,
		textContents,
		uploadStates,
		previewUrls,
		handleAttach,
		handleRemoveAttachment,
		startUpload,
		resetAttachments,
		// Raw setters bypass localStorage persistence; only use
		// when persist is false (e.g. ChatPageContent pre-populating
		// attachments from existing chat messages).
		setAttachments,
		setPreviewUrls,
		setUploadStates,
	};
}
