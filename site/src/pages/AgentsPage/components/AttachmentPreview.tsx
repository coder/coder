import { AlertTriangleIcon, ClipboardPasteIcon } from "lucide-react";
import type { FC, ReactEventHandler, ReactNode } from "react";
import { toast } from "sonner";
import { cn } from "#/utils/cn";
import { useLatestAbortController } from "../hooks/useLatestAbortController";
import { isAbortError } from "../utils/chatAttachments";
import {
	fetchTextAttachmentContent,
	formatTextAttachmentPreview,
	getTextAttachmentErrorMessage,
} from "../utils/fetchTextAttachment";
import { FileAttachmentTile } from "./FileAttachmentTile";

export type UploadState = {
	// "processing" covers any pre-upload client work (e.g. resize),
	// so paste/drop handlers can commit the attachment synchronously
	// without the send gate believing it is ready to dispatch.
	status: "pending" | "processing" | "uploading" | "uploaded" | "error";
	fileId?: string;
	error?: string;
	draftWarning?: string;
};

export const isUploadInProgress = (state: UploadState | undefined): boolean =>
	state?.status === "pending" ||
	state?.status === "processing" ||
	state?.status === "uploading";

/** Renders an image thumbnail from a pre-created preview URL. */
export const ImageThumbnail: FC<{
	previewUrl: string;
	name: string;
	className?: string;
	onError?: ReactEventHandler<HTMLImageElement>;
}> = ({ previewUrl, name, className, onError }) => (
	<img
		src={previewUrl}
		alt={name}
		className={cn(
			"h-16 w-16 rounded-md border border-border-default object-cover",
			className,
		)}
		onError={onError}
	/>
);

/** Renders a horizontal strip of attachment thumbnails above the input. */
export const AttachmentPreview: FC<{
	attachments: readonly File[];
	onRemove: (attachment: number | File) => void;
	uploadStates?: Map<File, UploadState>;
	previewUrls?: Map<File, string>;
	onPreview?: (url: string) => void;
	textContents?: Map<File, string>;
	onTextPreview?: (
		content: string,
		fileName: string,
		mediaType?: string,
	) => void;
	onInlineText?: (file: File, content?: string) => void;
}> = ({
	attachments,
	onRemove,
	uploadStates,
	previewUrls,
	onPreview,
	textContents,
	onTextPreview,
	onInlineText,
}) => {
	const textAttachmentRequest = useLatestAbortController();

	if (attachments.length === 0) return null;

	const loadTextAttachmentContent = async (
		content: string | undefined,
		fileId: string | undefined,
	): Promise<string | undefined> => {
		textAttachmentRequest.abort();
		if (content !== undefined || !fileId) {
			return content;
		}
		const controller = textAttachmentRequest.start();
		try {
			const result = await fetchTextAttachmentContent(
				fileId,
				controller.signal,
			);
			if (!textAttachmentRequest.clear(controller)) {
				return undefined;
			}
			if (result.kind === "loaded") {
				return result.content;
			}
			const resultMessage = getTextAttachmentErrorMessage(result);
			if (resultMessage !== null) {
				toast.error(resultMessage);
			}
			return undefined;
		} catch (err) {
			if (!textAttachmentRequest.clear(controller)) {
				return undefined;
			}
			if (isAbortError(err)) {
				return undefined;
			}
			const errorMessage = getTextAttachmentErrorMessage(err);
			if (errorMessage === null) {
				return undefined;
			}
			console.error("Failed to load text attachment:", err);
			toast.error(errorMessage);
			return undefined;
		}
	};

	const draftWarnings = Array.from(
		new Set(
			attachments.flatMap((file) => {
				const warning = uploadStates?.get(file)?.draftWarning;
				return warning ? [warning] : [];
			}),
		),
	);

	return (
		<div className="border-b border-border-default/50">
			<div className="flex gap-2 overflow-x-auto px-3 py-2">
				{attachments.map((file, index) => {
					const uploadState = uploadStates?.get(file);
					const previewUrl = previewUrls?.get(file) ?? "";
					const textContent = textContents?.get(file);
					const textFileId =
						uploadState?.status === "uploaded" ? uploadState.fileId : undefined;
					const hasTextAttachment =
						file.type === "text/plain" &&
						(textContent !== undefined || textFileId !== undefined);
					const isBusy =
						uploadState?.status === "pending" ||
						uploadState?.status === "processing" ||
						uploadState?.status === "uploading";
					const preview: ReactNode =
						file.type.startsWith("image/") && previewUrl ? (
							<ImageThumbnail
								previewUrl={previewUrl}
								name={file.name}
								className="h-10 w-10"
							/>
						) : hasTextAttachment ? (
							<span className="line-clamp-2 w-10 font-mono text-2xs text-content-secondary">
								{formatTextAttachmentPreview(textContent ?? "")}
							</span>
						) : undefined;
					const onClick =
						file.type.startsWith("image/") && previewUrl
							? () => onPreview?.(previewUrl)
							: hasTextAttachment
								? async () => {
										const nextContent = await loadTextAttachmentContent(
											textContent,
											textFileId,
										);
										if (nextContent !== undefined) {
											onTextPreview?.(nextContent, file.name, file.type);
										}
									}
								: undefined;
					const inlineAction = hasTextAttachment ? (
						<button
							type="button"
							onClick={async (event) => {
								event.stopPropagation();
								const nextContent = await loadTextAttachmentContent(
									textContent,
									textFileId,
								);
								onInlineText?.(file, nextContent);
							}}
							className="inline-flex size-6 items-center justify-center rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link"
							aria-label="Paste inline"
						>
							<ClipboardPasteIcon aria-hidden="true" className="size-3.5" />
						</button>
					) : undefined;

					return (
						<FileAttachmentTile
							key={`${file.name}-${file.size}-${file.lastModified}-${index}`}
							name={file.name}
							size={file.size}
							mediaType={file.type}
							preview={preview}
							onClick={onClick}
							extraActions={inlineAction}
							isUploading={isBusy}
							errorMessage={
								uploadState?.status === "error" ? uploadState.error : undefined
							}
							onRemove={() => onRemove(file)}
						/>
					);
				})}
			</div>
			{draftWarnings.length > 0 && (
				<div className="space-y-1 px-3 pb-2 text-xs text-content-warning">
					{draftWarnings.map((warning) => (
						<div key={warning} className="flex items-start gap-1.5">
							<AlertTriangleIcon className="mt-0.5 h-3.5 w-3.5 shrink-0" />
							<span>{warning}</span>
						</div>
					))}
				</div>
			)}
		</div>
	);
};
