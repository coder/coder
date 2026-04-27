import {
	type Dispatch,
	type SetStateAction,
	useEffect,
	useEffectEvent,
	useState,
} from "react";
import { API } from "#/api/api";
import type { UploadState } from "../components/AgentChatInput";
import { getChatFileURL } from "../utils/chatAttachments";
import {
	formatAgentAttachmentTooLargeError,
	formatAgentAttachmentUploadError,
	maxAgentAttachmentSize,
	readAgentAttachmentText,
} from "../utils/fileAttachmentLimits";

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
	// When the org ID is not yet known (e.g. still loading), skip
	// restoration entirely so we don't accidentally prune valid
	// entries. The initializer only runs once, so the caller must
	// ensure the org ID is available before mounting the hook.
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

		// Prune entries that don't match the current org.
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
			// Synthetic File used as a Map key only. Its content is
			// never read because the existing file_id is reused at
			// send time.
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
	options?: { persist?: boolean },
): UseFileAttachmentsReturn {
	const persist = options?.persist ?? false;

	// Restore previously uploaded attachments from localStorage
	// when persistence is enabled. Computed once on first render.
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
				// Pre-warm the browser HTTP cache for images so the
				// timeline can render them instantly after send. We
				// intentionally skip text attachments because the
				// composer already has the text content locally.
				if (isImage) {
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

	const handleAttach = (files: File[]) => {
		setAttachments((prev) => [...prev, ...files]);
		setPreviewUrls((prev) => {
			const next = new Map(prev);
			for (const file of files) {
				if (file.type !== "text/plain") {
					next.set(file, URL.createObjectURL(file));
				}
			}
			return next;
		});
		// Read text content for preview, but skip oversized files.
		for (const file of files) {
			if (file.type === "text/plain" && file.size <= maxAgentAttachmentSize) {
				void readAgentAttachmentText(file)
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
		for (const file of files) {
			if (file.size > maxAgentAttachmentSize) {
				setUploadStates((prev) =>
					new Map(prev).set(file, {
						status: "error" as const,
						error: formatAgentAttachmentTooLargeError(file.size),
					}),
				);
			} else {
				startUpload(file);
			}
		}
	};

	const handleRemoveAttachment = (attachment: number | File) => {
		// Resolve the file to remove and perform localStorage side
		// effects before entering state updaters. React may call
		// updaters more than once (StrictMode, React Compiler), so
		// they must stay pure.
		const idx =
			typeof attachment === "number"
				? attachment
				: attachments.indexOf(attachment);
		const removed = idx >= 0 ? attachments[idx] : undefined;
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
		// Raw setters exposed for ChatPageContent to pre-populate
		// attachments from existing chat messages. These bypass
		// localStorage persistence. Only use when persist is false.
		setAttachments,
		setPreviewUrls,
		setUploadStates,
	};
}
