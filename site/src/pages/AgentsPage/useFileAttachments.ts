import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import {
	type Dispatch,
	type SetStateAction,
	useEffect,
	useRef,
	useState,
} from "react";
import type { UploadState } from "./AgentChatInput";

interface UseFileAttachmentsReturn {
	attachments: File[];
	uploadStates: Map<File, UploadState>;
	previewUrls: Map<File, string>;
	handleAttach: (files: File[]) => void;
	handleRemoveAttachment: (index: number) => void;
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
				const result = await API.uploadChatFile(file, organizationId);
				setUploadStates((prev) =>
					new Map(prev).set(file, {
						status: "uploaded",
						fileId: result.id,
					}),
				);
				// Pre-warm the browser HTTP cache so the timeline
				// can render this image instantly after send. The
				// server responds with Cache-Control: private,
				// immutable, so the <img src> never hits the
				// network again.
				void fetch(`/api/experimental/chats/files/${result.id}`);
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
				next.set(file, URL.createObjectURL(file));
			}
			return next;
		});
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

	const handleRemoveAttachment = (index: number) => {
		setAttachments((prev) => {
			const removed = prev[index];
			if (removed) {
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
			}
			return prev.filter((_, i) => i !== index);
		});
	};

	const resetAttachments = () => {
		for (const [, url] of previewUrlsRef.current) {
			if (url.startsWith("blob:")) URL.revokeObjectURL(url);
		}
		setPreviewUrls(new Map());
		setUploadStates(new Map());
		setAttachments([]);
	};

	return {
		attachments,
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
