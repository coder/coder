import type * as TypesGen from "#/api/typesGenerated";
import type { UploadState } from "../AttachmentPreview";
import type { ChatMessageInputRef } from "../ChatMessageInput/ChatMessageInput";

export type PreparedUserSubmission = {
	requestContent: readonly TypesGen.ChatInputPart[];
	optimisticContent: readonly TypesGen.ChatMessagePart[];
	skippedAttachmentErrors: number;
};

type EditorContentPart = ReturnType<
	ChatMessageInputRef["getContentParts"]
>[number];

const isChatFilePart = (
	part: TypesGen.ChatMessagePart,
): part is TypesGen.ChatFilePart => part.type === "file";

// Edit-mode attachments are synthetic empty Files whose names encode the
// original file-block index. Preserve that mapping so local removals do
// not shift inline-data fallbacks onto the wrong attachment.
const getExistingFileBlockIndex = (file: File): number | undefined => {
	if (file.size > 0) {
		return undefined;
	}
	const match = file.name.match(/^attachment-(\d+)\./);
	if (!match) {
		return undefined;
	}
	const parsedIndex = Number(match[1]);
	return Number.isInteger(parsedIndex) ? parsedIndex : undefined;
};

const toOptimisticAttachmentPart = (
	file: File,
	state: UploadState | undefined,
	fallbackBlock: TypesGen.ChatFilePart | undefined,
): TypesGen.ChatFilePart | undefined => {
	if (state?.status === "uploaded" && state.fileId) {
		return {
			type: "file",
			file_id: state.fileId,
			media_type:
				file.type || fallbackBlock?.media_type || "application/octet-stream",
		};
	}
	if (fallbackBlock && (fallbackBlock.file_id || fallbackBlock.data)) {
		return fallbackBlock;
	}
	return undefined;
};

/**
 * Converts user-authored input into both transport and render payloads.
 *
 * The request payload (`ChatInputPart[]`) is sent to the API, while the
 * optimistic payload (`ChatMessagePart[]`) is rendered locally before the
 * authoritative replacement message arrives from the server.
 */
export const prepareUserSubmission = ({
	editorParts,
	attachments,
	uploadStates,
	editingFileBlocks,
}: {
	editorParts: readonly EditorContentPart[];
	attachments: readonly File[];
	uploadStates: ReadonlyMap<File, UploadState>;
	editingFileBlocks?: readonly TypesGen.ChatMessagePart[];
}): PreparedUserSubmission => {
	const requestContent: TypesGen.ChatInputPart[] = [];
	const optimisticContent: TypesGen.ChatMessagePart[] = [];
	let skippedAttachmentErrors = 0;

	for (const part of editorParts) {
		if (part.type === "text") {
			if (!part.text.trim()) {
				continue;
			}
			requestContent.push({ type: "text", text: part.text });
			optimisticContent.push({ type: "text", text: part.text });
			continue;
		}
		const referencePart = {
			type: "file-reference" as const,
			file_name: part.reference.fileName,
			start_line: part.reference.startLine,
			end_line: part.reference.endLine,
			content: part.reference.content,
		};
		requestContent.push(referencePart);
		optimisticContent.push(referencePart);
	}

	const existingFileBlocks = (editingFileBlocks ?? []).filter(isChatFilePart);
	for (const [index, file] of attachments.entries()) {
		const uploadState = uploadStates.get(file);
		if (uploadState?.status === "error") {
			skippedAttachmentErrors += 1;
			continue;
		}
		if (uploadState?.status === "uploaded" && uploadState.fileId) {
			requestContent.push({ type: "file", file_id: uploadState.fileId });
		}
		const fallbackBlockIndex = getExistingFileBlockIndex(file) ?? index;
		const optimisticPart = toOptimisticAttachmentPart(
			file,
			uploadState,
			existingFileBlocks[fallbackBlockIndex],
		);
		if (optimisticPart) {
			optimisticContent.push(optimisticPart);
		}
	}

	return {
		requestContent,
		optimisticContent,
		skippedAttachmentErrors,
	};
};
