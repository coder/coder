import { AlertTriangleIcon, ClipboardPasteIcon, XIcon } from "lucide-react";
import type { FC, ReactEventHandler } from "react";
import { toast } from "sonner";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { useLatestAbortController } from "../hooks/useLatestAbortController";
import { isAbortError } from "../utils/chatAttachments";
import {
	fetchTextAttachmentContent,
	formatTextAttachmentPreview,
	getTextAttachmentErrorMessage,
} from "../utils/fetchTextAttachment";

export type UploadState = {
	status: "pending" | "uploading" | "uploaded" | "error";
	fileId?: string;
	error?: string;
	draftWarning?: string;
};

export const isUploadInProgress = (state: UploadState | undefined): boolean =>
	state?.status === "pending" || state?.status === "uploading";

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
					return (
						<div
							// Key combines file metadata with index as a fallback for
							// duplicate names. Acceptable for a small, append-only list.
							key={`${file.name}-${file.size}-${file.lastModified}-${index}`}
							className="group relative"
						>
							{file.type.startsWith("image/") && previewUrl ? (
								<button
									type="button"
									className="border-0 bg-transparent p-0 cursor-pointer transition-opacity hover:opacity-80"
									onClick={() => onPreview?.(previewUrl)}
								>
									<ImageThumbnail previewUrl={previewUrl} name={file.name} />
								</button>
							) : hasTextAttachment ? (
								<button
									type="button"
									aria-label={`View ${file.name}`}
									className="flex h-16 w-28 flex-col items-start justify-start overflow-hidden rounded-md border-0 bg-surface-tertiary p-2 text-left transition-colors hover:bg-surface-quaternary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link"
									onClick={async () => {
										const nextContent = await loadTextAttachmentContent(
											textContent,
											textFileId,
										);
										if (nextContent !== undefined) {
											onTextPreview?.(nextContent, file.name, file.type);
										}
									}}
								>
									<span className="line-clamp-3 w-full font-mono text-2xs text-content-secondary">
										{formatTextAttachmentPreview(textContent ?? "")}
									</span>
								</button>
							) : (
								<div className="flex h-16 w-16 items-center justify-center rounded-md border border-border-default bg-surface-secondary text-xs text-content-secondary">
									{file.name.split(".").pop()?.toUpperCase() || "FILE"}
								</div>
							)}
							{hasTextAttachment && (
								<button
									type="button"
									onClick={async () => {
										const nextContent = await loadTextAttachmentContent(
											textContent,
											textFileId,
										);
										onInlineText?.(file, nextContent);
									}}
									className="absolute -bottom-2 -right-2 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full border-0 bg-surface-primary text-content-secondary shadow-sm opacity-0 transition-opacity hover:bg-surface-secondary hover:text-content-primary group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100"
									aria-label="Paste inline"
								>
									<ClipboardPasteIcon
										aria-hidden="true"
										className="h-3.5 w-3.5"
									/>
								</button>
							)}
							{(uploadState?.status === "pending" ||
								uploadState?.status === "uploading") && (
								<div className="absolute inset-0 flex items-center justify-center rounded-md bg-overlay">
									<Spinner className="h-5 w-5 text-white" loading />
								</div>
							)}
							{uploadState?.status === "error" && (
								<Tooltip>
									<TooltipTrigger asChild>
										<div
											className="absolute inset-0 flex items-center justify-center rounded-md bg-overlay"
											role="img"
											aria-label="Upload error"
										>
											<AlertTriangleIcon className="h-5 w-5 text-content-warning" />
										</div>
									</TooltipTrigger>
									<TooltipContent side="top">
										<p className="max-w-xs text-xs">
											{uploadState.error ?? "Upload failed"}
										</p>
									</TooltipContent>
								</Tooltip>
							)}
							<button
								type="button"
								onClick={() => onRemove(file)}
								className="absolute -right-2 -top-2 flex h-6 w-6 cursor-pointer items-center justify-center rounded-full border-0 bg-surface-primary text-content-secondary shadow-sm opacity-0 transition-opacity hover:bg-surface-secondary hover:text-content-primary group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100"
								aria-label={`Remove ${file.name}`}
							>
								<XIcon aria-hidden="true" className="h-3.5 w-3.5" />
							</button>
						</div>
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
