import { useRef, useState } from "react";
import { API } from "#/api/api";
import {
	MaxWorkspaceFileSizeBytes,
	type UploadChatWorkspaceFileResponse,
} from "#/api/typesGenerated";
import { renameChatFileForUpload } from "../utils/chatAttachments";
import {
	formatAgentAttachmentUploadError,
	formatWorkspaceUploadTooLargeError,
} from "../utils/fileAttachmentLimits";

export type WorkspaceUploadState =
	| { status: "uploading" }
	| {
			status: "uploaded";
			path: string;
			name: string;
			size: number;
			mediaType: string;
	  }
	| { status: "error"; error: string };

export const isWorkspaceUploadInProgress = (
	state: WorkspaceUploadState | undefined,
): boolean => state?.status === "uploading";

interface UseWorkspaceFileUploadsReturn {
	files: File[];
	uploadStates: Map<File, WorkspaceUploadState>;
	handleAttach: (files: File[]) => void;
	handleRemove: (file: File | number) => void;
	reset: () => void;
}

export function useWorkspaceFileUploads(
	chatId: string | undefined,
): UseWorkspaceFileUploadsReturn {
	const [files, setFiles] = useState<File[]>([]);
	const [uploadStates, setUploadStates] = useState(
		() => new Map<File, WorkspaceUploadState>(),
	);
	// Per-file AbortController so removing a file mid-upload cancels
	// the request without waiting for it to finish.
	const abortControllersRef = useRef(new Map<File, AbortController>());

	const startUpload = (file: File) => {
		if (!chatId) {
			setUploadStates((prev) =>
				new Map(prev).set(file, {
					status: "error",
					error: "Cannot upload: no active chat.",
				}),
			);
			return;
		}
		const controller = new AbortController();
		abortControllersRef.current.set(file, controller);
		setUploadStates((prev) => new Map(prev).set(file, { status: "uploading" }));
		void (async () => {
			let result: UploadChatWorkspaceFileResponse;
			try {
				result = await API.experimental.uploadChatWorkspaceFile(
					chatId,
					file,
					controller.signal,
				);
			} catch (err: unknown) {
				abortControllersRef.current.delete(file);
				if (controller.signal.aborted) {
					return;
				}
				const errorMessage = formatAgentAttachmentUploadError(err);
				setUploadStates((prev) =>
					new Map(prev).set(file, {
						status: "error",
						error: errorMessage,
					}),
				);
				return;
			}
			abortControllersRef.current.delete(file);
			setUploadStates((prev) =>
				new Map(prev).set(file, {
					status: "uploaded",
					path: result.path,
					name: result.name,
					size: result.size,
					mediaType: result.media_type,
				}),
			);
		})();
	};

	const handleAttach = (incoming: File[]) => {
		const renamed = incoming.map(renameChatFileForUpload);
		setFiles((prev) => [...prev, ...renamed]);
		const tooLargeErrors: Array<[File, string]> = [];
		for (const file of renamed) {
			if (file.size > MaxWorkspaceFileSizeBytes) {
				tooLargeErrors.push([
					file,
					formatWorkspaceUploadTooLargeError(file.size),
				]);
			}
		}
		if (tooLargeErrors.length > 0) {
			setUploadStates((prev) => {
				const next = new Map(prev);
				for (const [file, error] of tooLargeErrors) {
					next.set(file, { status: "error", error });
				}
				return next;
			});
		}
		for (const file of renamed) {
			if (file.size > MaxWorkspaceFileSizeBytes) continue;
			startUpload(file);
		}
	};

	const handleRemove = (attachment: File | number) => {
		setFiles((prev) => {
			const idx =
				typeof attachment === "number" ? attachment : prev.indexOf(attachment);
			if (idx === -1) return prev;
			const removed = prev[idx];
			abortControllersRef.current.get(removed)?.abort();
			abortControllersRef.current.delete(removed);
			setUploadStates((states) => {
				const next = new Map(states);
				next.delete(removed);
				return next;
			});
			return prev.filter((_, i) => i !== idx);
		});
	};

	const reset = () => {
		for (const controller of abortControllersRef.current.values()) {
			controller.abort();
		}
		abortControllersRef.current.clear();
		setFiles([]);
		setUploadStates(new Map());
	};

	return { files, uploadStates, handleAttach, handleRemove, reset };
}
