import {
	type Dispatch,
	type SetStateAction,
	useEffect,
	useEffectEvent,
	useRef,
	useState,
} from "react";
import { API } from "#/api/api";
import type { UploadState } from "../components/AgentChatInput";
import { getChatFileURL } from "../utils/chatAttachments";
import { formatAgentAttachmentUploadError } from "../utils/fileAttachmentLimits";
import { resizeImageToMaxBytes } from "../utils/resizeImage";

// Maximum bytes accepted by our upload endpoint (maxChatFileSize in
// coderd/exp_chats.go). Non-image files over this limit surface as
// an error; oversized images get resized down instead.
const MAX_UPLOAD_BYTES = 10 * 1024 * 1024;

// Default image budget keeps us safely under MAX_UPLOAD_BYTES after
// any trailing encoder metadata. Images larger than this are
// re-encoded before upload so we never have to reject them.
const DEFAULT_IMAGE_BUDGET_BYTES = MAX_UPLOAD_BYTES - 16 * 1024;

// Anthropic's documented inline image budget is 5 MiB (5,242,880
// bytes). Stay slightly below to leave room for framing on the
// wire. This is stricter than DEFAULT_IMAGE_BUDGET_BYTES, so the
// Anthropic path uses it even though the upload endpoint itself
// would accept more.
//
// Kept in sync with coderd/x/chatd/chatprovider/chatprovider.go
// `anthropicInlineImageByteCap` (server-side backstop) — adjust the
// byte values together if Anthropic ever revises the limit.
const ANTHROPIC_IMAGE_BUDGET_BYTES = 5 * 1024 * 1024 - 16 * 1024;

// Providers that inherit Anthropic's stricter 5 MiB inline image cap.
// Must mirror chatprovider.InlineImageByteCap's coverage on the
// server so both layers agree on which configurations need the
// tighter budget. Bedrock is included because fantasy's bedrock
// provider is a thin wrapper around the Anthropic client.
const ANTHROPIC_STRICT_BUDGET_PROVIDERS: ReadonlySet<string> = new Set([
	"anthropic",
	"bedrock",
]);

function imageBudgetForProvider(provider: string | undefined): number {
	if (provider && ANTHROPIC_STRICT_BUDGET_PROVIDERS.has(provider)) {
		return ANTHROPIC_IMAGE_BUDGET_BYTES;
	}
	return DEFAULT_IMAGE_BUDGET_BYTES;
}

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
	options?: { persist?: boolean; provider?: string },
): UseFileAttachmentsReturn {
	const persist = options?.persist ?? false;

	// Provider is read inside async callbacks; keep it in a ref so
	// the latest value is visible without rebuilding handleAttach
	// whenever the user switches models. Writing the ref from an
	// effect (rather than during render) keeps React Compiler happy
	// while still ensuring subsequent event-driven calls see the
	// newest value.
	const provider = options?.provider;
	const providerRef = useRef(provider);
	useEffect(() => {
		providerRef.current = provider;
	}, [provider]);

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

	// Track originals that were removed while their resize was still
	// in flight. Writes happen in handleRemoveAttachment; reads in
	// processResizes. A WeakSet lets dismissed File objects be GC'd
	// without needing explicit cleanup.
	const abandonedResizesRef = useRef<WeakSet<File>>(new WeakSet());

	const processResizes = async (
		originals: File[],
		needsResize: boolean[],
		budget: number,
	) => {
		// Resize images one at a time (module-level queue in
		// resizeImageToMaxBytes already serializes decode work; this
		// mirrors it so each swap is committed before the next
		// starts, keeping UI transitions predictable).
		for (let i = 0; i < originals.length; i++) {
			if (!needsResize[i]) continue;
			const original = originals[i];
			let resized: File | null = null;
			try {
				resized = await resizeImageToMaxBytes(original, budget);
			} catch {
				// Never let a resize error swallow the attachment;
				// we fall back to the original below.
				resized = null;
			}

			// If the user removed this attachment while resize was
			// in flight, skip state updates entirely so we don't
			// resurrect a dismissed file.
			if (abandonedResizesRef.current.has(original)) {
				continue;
			}
			const replacement = resized ?? original;
			const replaced = replacement !== original;

			// Functional updaters so a racing removal (e.g. unmount
			// cleanup) leaves the state alone: if the original is no
			// longer present, every updater below is a no-op.
			setAttachments((prev) => {
				const idx = prev.indexOf(original);
				if (idx === -1 || !replaced) return prev;
				const next = prev.slice();
				next[idx] = replacement;
				return next;
			});
			setPreviewUrls((prev) => {
				if (!prev.has(original)) return prev;
				const next = new Map(prev);
				const oldUrl = next.get(original);
				if (oldUrl?.startsWith("blob:")) URL.revokeObjectURL(oldUrl);
				next.delete(original);
				if (replaced && replacement.type !== "text/plain") {
					next.set(replacement, URL.createObjectURL(replacement));
				}
				return next;
			});
			setUploadStates((prev) => {
				if (!prev.has(original)) return prev;
				const next = new Map(prev);
				next.delete(original);
				return next;
			});

			// If the replacement is still larger than the server
			// cap (e.g. resize failed and the original was >10 MiB),
			// surface the pre-existing too-large error instead of
			// kicking off an upload that will 413.
			if (replacement.size > MAX_UPLOAD_BYTES) {
				setUploadStates((prev) =>
					new Map(prev).set(replacement, {
						status: "error" as const,
						error: `File too large (${(replacement.size / 1024 / 1024).toFixed(1)} MB). Maximum is 10 MB.`,
					}),
				);
				continue;
			}
			startUpload(replacement);
		}
	};

	const handleAttach = (files: File[]) => {
		// Commit originals to state synchronously so the composer
		// reflects the paste/drop immediately and the send gate
		// (via the "processing" UploadState) blocks dispatch until
		// any async preprocessing finishes. The async worker below
		// later swaps each entry for its resized File if needed.
		const budget = imageBudgetForProvider(providerRef.current);
		const needsResize = files.map(
			(file) => file.type.startsWith("image/") && file.size > budget,
		);

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
		setUploadStates((prev) => {
			const next = new Map(prev);
			for (let i = 0; i < files.length; i++) {
				const file = files[i];
				if (file.size > MAX_UPLOAD_BYTES && !needsResize[i]) {
					next.set(file, {
						status: "error" as const,
						error: `File too large (${(file.size / 1024 / 1024).toFixed(1)} MB). Maximum is 10 MB.`,
					});
				} else if (needsResize[i]) {
					next.set(file, { status: "processing" });
				}
			}
			return next;
		});

		// Read text content for preview, but skip oversized files.
		for (const file of files) {
			if (file.type === "text/plain" && file.size <= MAX_UPLOAD_BYTES) {
				// Defensive: some test environments lack File.prototype.text().
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

		// Start uploads for files that don't need resizing.
		for (let i = 0; i < files.length; i++) {
			const file = files[i];
			if (needsResize[i]) continue;
			if (file.size > MAX_UPLOAD_BYTES) continue; // already marked as error above
			startUpload(file);
		}

		// Kick off async resize + swap for files that need it.
		void processResizes(files, needsResize, budget);
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
		if (removed) {
			// Mark this file as abandoned so an in-flight resize
			// doesn't resurrect it by swapping in a replacement
			// after the state updates below.
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
