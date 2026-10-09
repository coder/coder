import { toast } from "sonner";
import { isWorkspaceUploadInProgress } from "../../hooks/useWorkspaceFileUploads";
import { isChatAttachmentFile } from "../../utils/chatAttachments";
import { isUploadInProgress } from "../AttachmentPreview";
import type { AgentComposerBindings } from "./context";

/** Routes files to their upload owner and computes submission readiness. */
export function composerFiles(
	bindings: AgentComposerBindings,
	resetPromptCycle: () => void,
) {
	const files = bindings.files;

	if (!files) {
		return {
			hasActiveUploads: false,
			hasUploadedAttachments: false,
			attachFiles: () => false,
		};
	}

	const { attachments, uploadStates, workspaceUploads } = files;
	const hasAttachmentUploads = attachments.some((file) =>
		isUploadInProgress(uploadStates.get(file)),
	);
	const hasWorkspaceUploads = workspaceUploads.uploads.some(
		isWorkspaceUploadInProgress,
	);
	const hasUploadedChatFiles = attachments.some(
		(file) => uploadStates.get(file)?.status === "uploaded",
	);
	const hasUploadedWorkspaceFiles = workspaceUploads.uploads.some((upload) => {
		if (upload.status === "uploaded" || upload.status === "deferred") {
			return true;
		}

		// Deferred failures retry on the next send; eager failures cannot be sent.
		return workspaceUploads.deferred === true && upload.status === "error";
	});

	const attachFiles = (incoming: File[]): boolean => {
		if (bindings.isReadOnly || !files.onAttach) {
			return false;
		}

		// Submission cleanup owns the current bucket, including files added mid-send.
		if (bindings.isLoading) {
			toast.error(
				"Wait for the current message to finish sending, then add the file again.",
			);
			return false;
		}

		const attachable: File[] = [];
		const forWorkspace: File[] = [];

		for (const file of incoming) {
			if (isChatAttachmentFile(file)) {
				attachable.push(file);
			} else {
				forWorkspace.push(file);
			}
		}

		let attached = false;

		if (attachable.length > 0) {
			files.onAttach(attachable);
			attached = true;
		}

		if (forWorkspace.length > 0) {
			if (workspaceUploads.onAttach && !bindings.isDisabled) {
				workspaceUploads.onAttach(forWorkspace);
				attached = true;
			} else {
				toast.error(
					workspaceUploads.unavailableMessage ??
						"This file type is uploaded into the chat's workspace. Attach a running workspace to the chat, then try again.",
				);
			}
		}

		if (attached) {
			resetPromptCycle();
		}

		return attached;
	};

	return {
		hasActiveUploads: hasAttachmentUploads || hasWorkspaceUploads,
		hasUploadedAttachments: hasUploadedChatFiles || hasUploadedWorkspaceFiles,
		attachFiles,
	};
}
