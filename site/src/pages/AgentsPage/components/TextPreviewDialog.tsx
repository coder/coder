import type { FC } from "react";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { Response } from "./ChatElements/Response";

interface TextPreviewDialogProps {
	content: string;
	fileName?: string;
	/** Explicit media type for the attachment, if known. */
	mediaType?: string;
	onClose: () => void;
}

/**
 * Returns true when the attachment should render as Markdown rather than as
 * a monospaced code block. We trust an explicit `text/markdown` media type
 * when available and otherwise fall back to the file extension, which is
 * how attached `.md` files arrive from the OS file picker.
 */
const isMarkdownPreview = (
	fileName: string | undefined,
	mediaType: string | undefined,
): boolean => {
	if (mediaType === "text/markdown") {
		return true;
	}
	if (!fileName) {
		return false;
	}
	const lower = fileName.toLowerCase();
	return lower.endsWith(".md") || lower.endsWith(".markdown");
};

export const TextPreviewDialog: FC<TextPreviewDialogProps> = ({
	content,
	fileName,
	mediaType,
	onClose,
}) => {
	const renderAsMarkdown = isMarkdownPreview(fileName, mediaType);

	return (
		<Dialog open onOpenChange={(open) => !open && onClose()}>
			<DialogContent
				className="max-h-[85vh] max-w-[90vw] w-full sm:w-fit sm:min-w-[400px] flex flex-col gap-0 p-0"
				aria-describedby={undefined}
			>
				<DialogTitle className="px-4 py-3 border-b border-border-default text-sm font-medium">
					{fileName ?? "Pasted text"}
				</DialogTitle>
				<div className="overflow-auto p-4 max-h-[calc(85vh-3rem)]">
					{renderAsMarkdown ? (
						// Reuse the same Markdown renderer used for chat messages
						// so attached markdown previews look consistent with the
						// rest of the conversation.
						<Response className="max-w-3xl">{content}</Response>
					) : (
						<pre className="whitespace-pre-wrap break-words text-sm text-content-primary font-mono m-0">
							{content}
						</pre>
					)}
				</div>
			</DialogContent>
		</Dialog>
	);
};
