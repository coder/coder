import { toast } from "sonner";
import { isWorkspaceUploadInProgress } from "../../hooks/useWorkspaceFileUploads";
import {
	isChatAttachmentFile,
	shouldRouteFileToWorkspace,
} from "../../utils/chatAttachments";
import { isUploadInProgress } from "../AttachmentPreview";
import type { AgentComposerBindings } from "./context";

const workspaceRequiredAttachmentMessage =
	"This file type is uploaded into the chat's workspace. Attach a running workspace to the chat, then try again.";
const uploadPendingSendMessage =
	"Wait for the current message to finish sending, then add the file again.";

/** Routes files to their upload owner and computes submission readiness. */
export function composerFiles(
	bindings: AgentComposerBindings,
	resetPromptCycle: () => void,
) {
	const { isDisabled, isReadOnly, isLoading, files } = bindings;
	const { attachments, onAttach, uploadStates, workspaceUploads } = files;
	const workspaceUploadEntries = workspaceUploads?.uploads ?? [];

	const hasAttachmentUploads = attachments.some((file) =>
		isUploadInProgress(uploadStates?.get(file)),
	);
	const hasWorkspaceUploads = workspaceUploadEntries.some(
		isWorkspaceUploadInProgress,
	);
	const hasUploadedChatFiles = attachments.some(
		(file) => uploadStates?.get(file)?.status === "uploaded",
	);
	const hasUploadedWorkspaceFiles = workspaceUploadEntries.some((upload) => {
		if (upload.status === "uploaded" || upload.status === "deferred") {
			return true;
		}

		// Deferred failures retry on the next send; eager failures cannot be sent.
		return workspaceUploads?.deferred === true && upload.status === "error";
	});

	// Workspace uploads eagerly write bytes into the workspace, so a
	// disabled composer must not route files to them.
	let onWorkspaceAttach: ((files: File[]) => void) | undefined;

	if (!isDisabled) {
		onWorkspaceAttach = workspaceUploads?.onAttach;
	}

	// Splits files between the attachment pipeline and the workspace
	// upload path by declared MIME type. Unknown or octet-stream files
	// stay on the attachment path where the server classifies bytes.
	const attachFiles = (incoming: File[]): boolean => {
		if (isReadOnly) {
			return false;
		}

		// The post-send reset clears every chat and workspace file, so files
		// added while a send is pending would be dropped after uploading.
		if (isLoading) {
			toast.error(uploadPendingSendMessage);
			return false;
		}

		const attachable: File[] = [];
		const forWorkspace: File[] = [];
		const rejected: File[] = [];
		// Workspace-routable files that arrived while the workspace
		// upload feature is wired but currently unavailable (no
		// connected agent, disabled composer). They need
		// a workspace, not a different file type, so they get their own
		// toast.
		const workspaceRequired: File[] = [];

		for (const file of incoming) {
			const routesToWorkspace = shouldRouteFileToWorkspace(file);

			if (onWorkspaceAttach && routesToWorkspace) {
				forWorkspace.push(file);
			} else if (isChatAttachmentFile(file)) {
				attachable.push(file);
			} else if (workspaceUploads && routesToWorkspace) {
				workspaceRequired.push(file);
			} else {
				rejected.push(file);
			}
		}

		if (workspaceRequired.length > 0) {
			toast.error(
				workspaceUploads?.unavailableMessage ??
					workspaceRequiredAttachmentMessage,
			);
		}

		if (rejected.length > 0) {
			const names = rejected.map((file) => file.name).join(", ");
			toast.error(`Unsupported file type: ${names}`);
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
