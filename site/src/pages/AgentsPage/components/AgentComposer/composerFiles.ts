import { toast } from "sonner";
import { isWorkspaceUploadInProgress } from "../../hooks/useWorkspaceFileUploads";
import { isChatAttachmentFile } from "../../utils/chatAttachments";
import { isUploadInProgress } from "../AttachmentPreview";
import type { AgentComposerBindings } from "./context";

const workspaceRequiredAttachmentMessage =
	"This file type is uploaded into the chat's workspace. Attach a running workspace to the chat, then try again.";
const workspaceUploadPendingSendMessage =
	"Wait for the current message to finish sending, then add the file again.";

/** Routes files to their upload owner and computes submission readiness. */
export function composerFiles(
	bindings: AgentComposerBindings,
	resetPromptCycle: () => void,
) {
	const { isDisabled, isLoading = false, files } = bindings;

	if (!files) {
		return {
			hasActiveUploads: false,
			hasUploadedAttachments: false,
			attachFiles: () => false,
		};
	}

	const { attachments, onAttach, uploadStates, workspaceUploads } = files;
	const workspaceUploadEntries = workspaceUploads.uploads;

	const hasAttachmentUploads = attachments.some((file) =>
		isUploadInProgress(uploadStates.get(file)),
	);
	const hasWorkspaceUploads = workspaceUploadEntries.some(
		isWorkspaceUploadInProgress,
	);
	const hasUploadedChatFiles = attachments.some(
		(file) => uploadStates.get(file)?.status === "uploaded",
	);
	const hasUploadedWorkspaceFiles = workspaceUploadEntries.some((upload) => {
		if (upload.status === "uploaded" || upload.status === "deferred") {
			return true;
		}

		// Deferred failures retry on the next send; eager failures cannot be sent.
		return workspaceUploads.deferred === true && upload.status === "error";
	});

	// Workspace uploads eagerly write bytes into the workspace, so a
	// disabled (read-only) composer must not route files to them. The
	// same holds while a send is pending: the post-send reset would drop
	// the chip after the bytes already landed in the workspace.
	const hasWorkspaceAttach = workspaceUploads.onAttach !== undefined;
	const workspaceAttachBlockedBySend = isLoading && hasWorkspaceAttach;
	let onWorkspaceAttach: ((files: File[]) => void) | undefined;

	if (!isDisabled && !isLoading) {
		onWorkspaceAttach = workspaceUploads.onAttach;
	}

	// Splits files between the attachment pipeline and the workspace
	// upload path by declared MIME type. Unknown or octet-stream files
	// stay on the attachment path where the server classifies bytes.
	const attachFiles = (incoming: File[]): boolean => {
		const attachable: File[] = [];
		const forWorkspace: File[] = [];
		// Workspace files that arrived while workspace uploads are
		// unavailable (no connected agent, read-only composer, pending
		// send). They need a workspace, not a different file type, so
		// they get their own toast.
		const workspaceRequired: File[] = [];

		for (const file of incoming) {
			if (isChatAttachmentFile(file)) {
				attachable.push(file);
			} else if (onWorkspaceAttach) {
				forWorkspace.push(file);
			} else {
				workspaceRequired.push(file);
			}
		}

		if (workspaceRequired.length > 0) {
			let message =
				workspaceUploads.unavailableMessage ??
				workspaceRequiredAttachmentMessage;

			if (workspaceAttachBlockedBySend) {
				message = workspaceUploadPendingSendMessage;
			}

			toast.error(message);
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

	return {
		hasActiveUploads: hasAttachmentUploads || hasWorkspaceUploads,
		hasUploadedAttachments: hasUploadedChatFiles || hasUploadedWorkspaceFiles,
		attachFiles,
	};
}
