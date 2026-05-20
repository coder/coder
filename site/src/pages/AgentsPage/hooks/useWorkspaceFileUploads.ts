import { useCallback, useEffect, useRef, useState } from "react";
import { API } from "#/api/api";
import type { UploadChatWorkspaceFileResponse } from "#/api/typesGenerated";
import { renameChatFileForUpload } from "../utils/chatAttachments";
import { formatAgentAttachmentUploadError } from "../utils/fileAttachmentLimits";

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

const newUploadStates = () => new Map<File, WorkspaceUploadState>();

export function useWorkspaceFileUploads(
	chatId: string | undefined,
): UseWorkspaceFileUploadsReturn {
	const [files, setFiles] = useState<File[]>([]);
	const [uploadStates, setUploadStates] = useState(newUploadStates);
	const previousChatIdRef = useRef(chatId);
	const uploadRunRef = useRef(0);
	const abortControllersRef = useRef(new Map<File, AbortController>());

	const abortAllUploads = useCallback(() => {
		for (const controller of abortControllersRef.current.values()) {
			controller.abort();
		}
		abortControllersRef.current.clear();
	}, []);

	const reset = useCallback(() => {
		uploadRunRef.current += 1;
		abortAllUploads();
		setFiles([]);
		setUploadStates(newUploadStates());
	}, [abortAllUploads]);

	useEffect(() => {
		return () => {
			abortAllUploads();
		};
	}, [abortAllUploads]);

	useEffect(() => {
		if (previousChatIdRef.current === chatId) {
			return;
		}
		previousChatIdRef.current = chatId;
		reset();
	}, [chatId, reset]);

	const setUploadState = useCallback(
		(file: File, uploadRun: number, state: WorkspaceUploadState) => {
			setUploadStates((prev) => {
				if (uploadRunRef.current !== uploadRun || !prev.has(file)) {
					return prev;
				}
				return new Map(prev).set(file, state);
			});
		},
		[],
	);

	const startUpload = useCallback(
		(file: File, uploadRun: number) => {
			if (!chatId) {
				setUploadStates((prev) =>
					new Map(prev).set(file, {
						status: "error",
						error: "Cannot upload: no active chat. Open or start a chat first.",
					}),
				);
				return;
			}

			const controller = new AbortController();
			abortControllersRef.current.set(file, controller);
			setUploadStates((prev) =>
				new Map(prev).set(file, { status: "uploading" }),
			);

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
					if (!controller.signal.aborted) {
						setUploadState(file, uploadRun, {
							status: "error",
							error: formatAgentAttachmentUploadError(err),
						});
					}
					return;
				}

				abortControllersRef.current.delete(file);
				setUploadState(file, uploadRun, {
					status: "uploaded",
					path: result.path,
					name: result.name,
					size: result.size,
					mediaType: result.media_type,
				});
			})();
		},
		[chatId, setUploadState],
	);

	const handleAttach = useCallback(
		(incoming: File[]) => {
			const renamed = incoming.map(renameChatFileForUpload);
			const uploadRun = uploadRunRef.current;
			setFiles((prev) => [...prev, ...renamed]);
			for (const file of renamed) {
				startUpload(file, uploadRun);
			}
		},
		[startUpload],
	);

	const handleRemove = useCallback(
		(attachment: File | number) => {
			const index =
				typeof attachment === "number" ? attachment : files.indexOf(attachment);
			if (index === -1) {
				return;
			}

			const removed = files[index];
			abortControllersRef.current.get(removed)?.abort();
			abortControllersRef.current.delete(removed);
			setUploadStates((states) => {
				const next = new Map(states);
				next.delete(removed);
				return next;
			});
			setFiles((prev) => prev.filter((_, i) => i !== index));
		},
		[files],
	);

	return { files, uploadStates, handleAttach, handleRemove, reset };
}
