import type * as TypesGen from "#/api/typesGenerated";
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

export const buildAttachmentMediaTypes = (
	attachments?: readonly PendingAttachment[],
): ReadonlyMap<string, string> | undefined => {
	if (!attachments?.length) {
		return undefined;
	}

	return new Map(
		attachments.map(({ fileId, mediaType }) => [fileId, mediaType]),
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
		for (const { fileId } of attachments) {
			content.push({ type: "file", file_id: fileId });
		}
	}

	if (workspaceUploads && workspaceUploads.length > 0) {
		for (const upload of workspaceUploads) {
			content.push(toWorkspaceFileReferencePart(upload));
		}
	}

	return { content, hasContent: content.length > 0 };
};
