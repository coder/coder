import type React from "react";
import { toast } from "sonner";
import { isWorkspaceUploadInProgress } from "../../hooks/useWorkspaceFileUploads";
import {
	isChatAttachmentFile,
	shouldRouteFileToWorkspace,
} from "../../utils/chatAttachments";
import { isUploadInProgress } from "../AttachmentPreview";
import type { ChatMessageInputRef } from "../ChatMessageInput/ChatMessageInput";
import type { AgentComposerBindings } from "./context";

/** Routes files to their upload owner and computes submission readiness. */
export function useComposerFiles(
	bindings: AgentComposerBindings,
	editorRef: React.RefObject<ChatMessageInputRef | null>,
	resetPromptCycle: () => void,
) {
	const { isDisabled, isLoading } = bindings;
	const {
		attachments = [],
		onAttach,
		onRemoveAttachment,
		uploadStates,
		textContents,
		workspaceUploads,
	} = bindings.files ?? {};

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

	const attachFiles = (files: File[]): boolean => {
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

	return {
		hasActiveUploads,
		hasUploadedAttachments,
		attachFiles,
		inlineText,
	};
}
