import type { UploadState } from "../components/AttachmentPreview";
import {
	formatInlinedAttachmentText,
	isInlinableTextAttachment,
	type PendingAttachment,
} from "./chatAttachments";
import { readAgentAttachmentText } from "./fileAttachmentLimits";

type ResolvedAttachments = {
	attachments: PendingAttachment[];
	// Count of attachments skipped because their upload failed, so callers
	// can surface a single aggregated warning.
	skippedErrors: number;
};

/**
 * Resolves uploaded attachments into the metadata sent at message submission.
 *
 * Text-family uploads are inlined as text so providers that reject the media
 * type as a file part still see the content. The content read at attach time
 * is preferred; otherwise it is read on demand. When the on-demand read fails
 * the attachment falls back to a file part rather than dropping the message.
 * Restored attachments carry no bytes (size 0), so they keep the file part.
 *
 * Shared by the chat page and the create form so both submit paths stay in
 * sync.
 */
export async function resolvePendingAttachments(
	files: readonly File[],
	uploadStates: Map<File, UploadState>,
	textContents: Map<File, string>,
): Promise<ResolvedAttachments> {
	const attachments: PendingAttachment[] = [];
	let skippedErrors = 0;

	for (const file of files) {
		const state = uploadStates.get(file);
		if (state?.status === "error") {
			skippedErrors++;
			continue;
		}
		if (state?.status !== "uploaded" || !state.fileId) {
			continue;
		}

		const attachment: PendingAttachment = {
			fileId: state.fileId,
			mediaType: file.type || "application/octet-stream",
		};

		if (isInlinableTextAttachment(file)) {
			let content = textContents.get(file);
			if (!content && file.size > 0) {
				try {
					content = await readAgentAttachmentText(file);
				} catch (err) {
					// Fall back to the file part so the message still sends.
					console.warn(
						"Failed to inline text attachment; sending as a file part:",
						err,
					);
				}
			}
			if (content) {
				attachment.textContent = formatInlinedAttachmentText(
					file.name,
					content,
				);
			}
		}

		attachments.push(attachment);
	}

	return { attachments, skippedErrors };
}
