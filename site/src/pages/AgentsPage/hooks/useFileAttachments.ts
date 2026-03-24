import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import {
	type Dispatch,
	type SetStateAction,
	useEffect,
	useRef,
	useState,
} from "react";
import type { UploadState } from "../components/AgentChatInput";

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
): UseFileAttachmentsReturn {
	const [attachments, setAttachments] = useState<File[]>([]);
	const [uploadStates, setUploadStates] = useState(
		() => new Map<File, UploadState>(),
	);
	const [previewUrls, setPreviewUrls] = useState(() => new Map<File, string>());
	const [textContents, setTextContents] = useState(
		() => new Map<File, string>(),
	);

	// Revoke blob URLs on unmount to prevent memory leaks.
	const previewUrlsRef = useRef(previewUrls);
	useEffect(() => {
		previewUrlsRef.current = previewUrls;
	});
	useEffect(() => {
		return () => {
			for (const [, url] of previewUrlsRef.current) {
				if (url.startsWith("blob:")) URL.revokeObjectURL(url);
			}
		};
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
				// Pre-warm the browser HTTP cache for images so the
				// timeline can render them instantly after send. We
				// intentionally skip text attachments because the
				// composer already has the text content locally.
				if (file.type.startsWith("image/")) {
					void fetch(`/api/experimental/chats/files/${result.id}`);
				}
			} catch (err: unknown) {
				const message = getErrorMessage(err, "Upload failed");
				const detail = getErrorDetail(err);
				const errorMessage = detail ? `${message} ${detail}` : message;
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
		const maxSize = 10 * 1024 * 1024; // 10 MB
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
			if (file.type === "text/plain" && file.size <= maxSize) {
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
		for (const file of files) {
			if (file.size > maxSize) {
				setUploadStates((prev) =>
					new Map(prev).set(file, {
						status: "error" as const,
						error: `File too large (${(file.size / 1024 / 1024).toFixed(1)} MB). Maximum is 10 MB.`,
					}),
				);
			} else {
				startUpload(file);
			}
		}
	};

	const handleRemoveAttachment = (attachment: number | File) => {
		setAttachments((prev) => {
			const index =
				typeof attachment === "number" ? attachment : prev.indexOf(attachment);
			if (index === -1) {
				return prev;
			}

			const removed = prev[index];
			setUploadStates((prevStates) => {
				const next = new Map(prevStates);
				next.delete(removed);
				return next;
			});
			setPreviewUrls((prevUrls) => {
				const url = prevUrls.get(removed);
				if (url?.startsWith("blob:")) URL.revokeObjectURL(url);
				const next = new Map(prevUrls);
				next.delete(removed);
				return next;
			});
			setTextContents((prevContents) => {
				const next = new Map(prevContents);
				next.delete(removed);
				return next;
			});
			return prev.filter((_, i) => i !== index);
		});
	};

	const resetAttachments = () => {
		for (const [, url] of previewUrlsRef.current) {
			if (url.startsWith("blob:")) URL.revokeObjectURL(url);
		}
		setPreviewUrls(new Map());
		setTextContents(new Map());
		setUploadStates(new Map());
		setAttachments([]);
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
		setAttachments,
		setPreviewUrls,
		setUploadStates,
	};
}
