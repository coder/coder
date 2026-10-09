import type React from "react";
import { useState } from "react";
import { toast } from "sonner";
import { isWorkspaceUploadInProgress } from "../../hooks/useWorkspaceFileUploads";
import {
	isChatAttachmentFile,
	shouldRouteFileToWorkspace,
} from "../../utils/chatAttachments";
import { isUploadInProgress } from "../AttachmentPreview";
import type { ChatMessageInputRef } from "../ChatMessageInput/ChatMessageInput";
import { ImageLightbox } from "../ImageLightbox";
import { TextPreviewDialog } from "../TextPreviewDialog";
import type { AgentComposerBindings } from "./context";

type TextPreview = { content: string; fileName: string; mediaType: string };

/** Handles file selection, upload readiness, and attachment previews. */
export function useComposerFiles(
	bindings: AgentComposerBindings,
	editorRef: React.RefObject<ChatMessageInputRef | null>,
	fileInputRef: React.RefObject<HTMLInputElement | null>,
	resetPromptCycle: () => void,
) {
	const {
		isDisabled,
		isLoading,
		attachments = [],
		onAttach,
		onRemoveAttachment,
		uploadStates,
		textContents,
		workspaceUploads,
	} = bindings;

	const [previewImage, setPreviewImage] = useState<string | null>(null);
	const [previewText, setPreviewText] = useState<TextPreview | null>(null);

	const [isDragging, setIsDragging] = useState(false);

	const workspaceUploadEntries = workspaceUploads?.uploads ?? [];
	const hasActiveUploads =
		attachments.some((file) => isUploadInProgress(uploadStates?.get(file))) ||
		workspaceUploadEntries.some(isWorkspaceUploadInProgress);

	// Deferred failures remain sendable because the next send re-uploads them.
	const hasUploadedAttachments =
		attachments.some((f) => uploadStates?.get(f)?.status === "uploaded") ||
		workspaceUploadEntries.some(
			(upload) =>
				upload.status === "uploaded" ||
				upload.status === "deferred" ||
				(workspaceUploads?.deferred === true && upload.status === "error"),
		);

	// Block new workspace uploads during submission so draft cleanup cannot discard them.
	const workspaceAttachBlockedBySend =
		isLoading && workspaceUploads?.onAttach !== undefined;
	const onWorkspaceAttach =
		isDisabled || isLoading ? undefined : workspaceUploads?.onAttach;

	const routeFiles = (files: File[]): boolean => {
		const attachable: File[] = [];
		const forWorkspace: File[] = [];
		const rejected: File[] = [];
		const workspaceRequired: File[] = [];

		for (const file of files) {
			if (onWorkspaceAttach && shouldRouteFileToWorkspace(file)) {
				forWorkspace.push(file);
			} else if (isChatAttachmentFile(file)) {
				attachable.push(file);
			} else if (workspaceUploads && shouldRouteFileToWorkspace(file)) {
				workspaceRequired.push(file);
			} else {
				rejected.push(file);
			}
		}

		if (workspaceRequired.length > 0) {
			if (workspaceAttachBlockedBySend) {
				toast.error(
					"Wait for the current message to finish sending, then add the file again.",
				);
			} else {
				toast.error(
					workspaceUploads?.unavailableMessage ??
						"This file type is uploaded into the chat's workspace. Attach a running workspace to the chat, then try again.",
				);
			}
		}

		if (rejected.length > 0) {
			toast.error(
				`Unsupported file type: ${rejected.map((file) => file.name).join(", ")}`,
			);
		}

		if (attachable.length === 0 && forWorkspace.length === 0) {
			return false;
		}

		resetPromptCycle();
		if (attachable.length > 0) {
			onAttach?.(attachable);
		}
		if (forWorkspace.length > 0) {
			onWorkspaceAttach?.(forWorkspace);
		}

		return true;
	};

	const fileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
		if (e.target.files?.length) {
			routeFiles(Array.from(e.target.files));
		}

		// Reset so the same file can be selected again.
		e.target.value = "";
	};

	const filePaste = (file: File) => routeFiles([file]);

	const openFilePicker = () => {
		resetPromptCycle();
		fileInputRef.current?.click();
	};

	const inlineText = (file: File, nextContent?: string) => {
		const content = nextContent ?? textContents?.get(file);

		if (content === undefined) {
			return;
		}

		const editor = editorRef.current;

		if (!editor) {
			return;
		}

		resetPromptCycle();
		editor.insertText(content);
		onRemoveAttachment?.(file);
	};

	const textPreview = (
		content: string,
		fileName: string,
		mediaType: string,
	) => {
		setPreviewText({ content, fileName, mediaType });
	};

	const dragOver = (e: React.DragEvent) => {
		e.preventDefault();

		if (e.dataTransfer.types.includes("Files")) {
			setIsDragging(true);
		}
	};

	const dragLeave = (e: React.DragEvent) => {
		if (
			!(e.relatedTarget instanceof Node) ||
			!e.currentTarget.contains(e.relatedTarget)
		) {
			setIsDragging(false);
		}
	};

	const drop = (e: React.DragEvent) => {
		e.preventDefault();
		setIsDragging(false);

		if (!e.dataTransfer.files.length) {
			return;
		}

		routeFiles(Array.from(e.dataTransfer.files));
	};

	return {
		hasActiveUploads,
		hasUploadedAttachments,
		isDragging,
		actions: {
			openFilePicker,
			fileSelect,
			filePaste,
			inlineText,
			textPreview,
			imagePreview: setPreviewImage,
			dragOver,
			dragLeave,
			drop,
		},
		previews: { previewImage, setPreviewImage, previewText, setPreviewText },
	};
}

/** Displays the attachment previews opened by file interactions. */
export function ComposerFilePreviews({
	previewImage,
	setPreviewImage,
	previewText,
	setPreviewText,
}: ReturnType<typeof useComposerFiles>["previews"]) {
	return (
		<>
			{previewImage && (
				<ImageLightbox
					src={previewImage}
					onClose={() => setPreviewImage(null)}
				/>
			)}
			{previewText !== null && (
				<TextPreviewDialog
					content={previewText.content}
					fileName={previewText.fileName}
					mediaType={previewText.mediaType}
					onClose={() => setPreviewText(null)}
				/>
			)}
		</>
	);
}
