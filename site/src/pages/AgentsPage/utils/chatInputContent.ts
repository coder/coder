import type * as TypesGen from "#/api/typesGenerated";
import type { UploadState } from "../components/AttachmentPreview";
import type {
	PendingAttachment,
	PendingWorkspaceUpload,
} from "../components/ChatPageContent";

export type ChatComposerContentPart =
	| { readonly type: "text"; readonly text: string }
	| {
			readonly type: "file-reference";
			readonly reference: {
				readonly fileName: string;
				readonly startLine: number;
				readonly endLine: number;
				readonly content: string;
			};
	  };

/** Failed uploads are skipped and counted; uploads still in flight are left out uncounted. */
export const collectSendableAttachments = (
	files: readonly File[],
	uploadStates: ReadonlyMap<File, UploadState>,
): { attachments: PendingAttachment[]; skippedErrors: number } => {
	const attachments: PendingAttachment[] = [];
	let skippedErrors = 0;
	for (const file of files) {
		const state = uploadStates.get(file);
		if (state?.status === "error") {
			skippedErrors++;
			continue;
		}
		if (state?.status === "uploaded" && state.fileId) {
			attachments.push({
				fileId: state.fileId,
				mediaType: file.type || "application/octet-stream",
				name: file.name,
				size: file.size,
				purpose: state.purpose,
			});
		}
	}
	return { attachments, skippedErrors };
};

/** The optimistic edited message reads each chip's name and size from here. */
export const buildPendingAttachmentsByID = (
	attachments?: readonly PendingAttachment[],
): ReadonlyMap<string, PendingAttachment> | undefined => {
	if (!attachments?.length) {
		return undefined;
	}

	return new Map(
		attachments.map((attachment) => [attachment.fileId, attachment]),
	);
};

export const toWorkspaceFileReferencePart = (
	upload: PendingWorkspaceUpload,
): TypesGen.ChatWorkspaceFileReferencePart => ({
	type: "workspace-file-reference",
	workspace_file_path: upload.path,
	workspace_file_name: upload.name,
	workspace_file_size: upload.size,
	workspace_file_media_type: upload.mediaType || "application/octet-stream",
	workspace_file_workspace_id: upload.workspaceId,
});

/**
 * When `composerParts` is provided, file-reference chips stay in document
 * order. Omit it to send `message` as text only.
 */
export const buildChatInputContent = ({
	message,
	attachments,
	workspaceUploads,
	composerParts,
}: {
	message: string;
	attachments?: readonly PendingAttachment[];
	workspaceUploads?: readonly PendingWorkspaceUpload[];
	composerParts?: readonly ChatComposerContentPart[];
}): { content: TypesGen.ChatInputPart[]; hasContent: boolean } => {
	const content: TypesGen.ChatInputPart[] = [];

	if (composerParts) {
		for (const part of composerParts) {
			if (part.type === "text") {
				if (part.text.trim()) {
					content.push({ type: "text", text: part.text });
				}
			} else {
				const reference = part.reference;
				content.push({
					type: "file-reference",
					file_name: reference.fileName,
					start_line: reference.startLine,
					end_line: reference.endLine,
					content: reference.content,
				});
			}
		}

		if (content.length === 0 && message.trim()) {
			content.push({ type: "text", text: message });
		}
	} else if (message.trim()) {
		content.push({ type: "text", text: message });
	}

	if (attachments && attachments.length > 0) {
		for (const { fileId, purpose } of attachments) {
			content.push({
				type: purpose === "workspace" ? "workspace-attachment" : "file",
				file_id: fileId,
			});
		}
	}

	if (workspaceUploads && workspaceUploads.length > 0) {
		for (const upload of workspaceUploads) {
			content.push(toWorkspaceFileReferencePart(upload));
		}
	}

	return { content, hasContent: content.length > 0 };
};
