import { AlertTriangleIcon } from "lucide-react";
import { type FC, useState } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { useLatestAbortController } from "../../hooks/useLatestAbortController";
import {
	type AttachmentFailure,
	attachmentFailureFromError,
	getChatFileURL,
	isAbortError,
	probeAttachmentFailure,
} from "../../utils/chatAttachments";
import {
	decodeInlineTextAttachment,
	fetchTextAttachmentContent,
	formatTextAttachmentPreview,
} from "../../utils/fetchTextAttachment";
import { ImageThumbnail } from "../AgentChatInput";
import { FileAttachmentTile } from "../FileAttachmentTile";
import { useFileProbes } from "./FileProbeContext";
import type { RenderBlock } from "./types";

export type PreviewTextAttachment = {
	content: string;
	fileName?: string;
	mediaType?: string;
};

type FileAttachmentBlock = Extract<RenderBlock, { type: "file" }>;

const TEXT_ATTACHMENT_MEDIA_TYPES = new Set([
	"text/plain",
	"text/markdown",
	"text/csv",
	"application/json",
]);

const ATTACHMENT_FALLBACK_EXTENSIONS: Record<string, string> = {
	"application/json": "json",
	"application/octet-stream": "bin",
	"application/pdf": "pdf",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation":
		"pptx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": "xlsx",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		"docx",
	"application/x-tar": "tar",
	"image/jpeg": "jpg",
	"text/markdown": "md",
	"text/plain": "txt",
};

const sanitizeAttachmentExtension = (value: string): string => {
	const sanitized = value
		.replace(/[^a-z0-9]/gi, "")
		.slice(0, 4)
		.toLowerCase();
	return sanitized || "file";
};

const getAttachmentExtension = (
	block: Pick<FileAttachmentBlock, "media_type" | "name">,
): string => {
	const mapped = ATTACHMENT_FALLBACK_EXTENSIONS[block.media_type];
	if (mapped) {
		return mapped;
	}
	const trimmedName = block.name?.trim();
	if (trimmedName) {
		const lastDot = trimmedName.lastIndexOf(".");
		// Keep dotfiles like `.env` out of the extension path, while still
		// allowing ordinary `name.ext` filenames to contribute a fallback.
		if (lastDot > 0 && lastDot < trimmedName.length - 1) {
			return sanitizeAttachmentExtension(trimmedName.slice(lastDot + 1));
		}
	}
	const subtype = block.media_type.split("/")[1] ?? "";
	if (subtype.endsWith("+json")) {
		return "json";
	}
	return sanitizeAttachmentExtension(subtype);
};

const isTextPreviewAttachmentMediaType = (mediaType: string): boolean =>
	TEXT_ATTACHMENT_MEDIA_TYPES.has(mediaType);

const getAttachmentHref = (block: FileAttachmentBlock): string | null => {
	if (block.file_id) {
		return getChatFileURL(block.file_id);
	}
	if (block.data) {
		return `data:${block.media_type};base64,${block.data}`;
	}
	return null;
};

const getAttachmentDisplayName = (
	block: Pick<FileAttachmentBlock, "media_type" | "name">,
): string => {
	const name = block.name?.trim();
	if (name) {
		return name;
	}
	if (block.media_type.startsWith("image/")) {
		return "Attached image";
	}
	if (isTextPreviewAttachmentMediaType(block.media_type)) {
		return "Pasted text";
	}
	return "Attached file";
};

const getAttachmentDownloadName = (
	block: Pick<FileAttachmentBlock, "media_type" | "name">,
): string => {
	const name = block.name?.trim();
	if (name) {
		return name;
	}
	const extension = getAttachmentExtension(block);
	return extension === "file" ? "attachment" : `attachment.${extension}`;
};

type AttachmentFailureState = { kind: "idle" } | AttachmentFailure;

type AttachmentFailureLabels = {
	expired: string;
	failed: string;
};

const imageAttachmentFailureLabels: AttachmentFailureLabels = {
	expired: "Image expired",
	failed: "Image failed to load",
};

const textAttachmentFailureLabels: AttachmentFailureLabels = {
	expired: "Attachment expired",
	failed: "Attachment failed to load",
};

const AttachmentFallbackTile: FC<{
	state: AttachmentFailure;
	labels: AttachmentFailureLabels;
	className?: string;
}> = ({ state, labels, className = "h-16 w-16" }) => {
	const label = state.kind === "expired" ? labels.expired : labels.failed;

	const tile = (
		<div
			role="img"
			aria-label={label}
			className={cn(
				"flex flex-col items-center justify-center gap-1 rounded-md border border-border-default bg-surface-tertiary px-1 text-center text-2xs text-content-secondary",
				className,
			)}
		>
			<AlertTriangleIcon
				className="size-icon-sm shrink-0 text-content-warning"
				aria-hidden="true"
			/>
			<span className="leading-tight">{label}</span>
		</div>
	);

	// Only surface a tooltip when we have something to add:
	// - "expired" explains the retention policy.
	// - "failed" with a detail surfaces the API error or network reason.
	// A bare "failed" (e.g. an inline base64 decode failure, where the
	// browser exposes nothing useful) stays a plain tile.
	const tooltipBody =
		state.kind === "expired"
			? "Chat attachments are deleted after the retention window set for this deployment."
			: state.detail;
	if (!tooltipBody) {
		return tile;
	}

	return (
		<Tooltip>
			<TooltipTrigger asChild>{tile}</TooltipTrigger>
			<TooltipContent side="top" className="max-w-xs">
				{tooltipBody}
			</TooltipContent>
		</Tooltip>
	);
};

const InlineTextAttachmentButton: FC<{
	content: string;
	fileName?: string;
	mediaType?: string;
	size?: number;
	href?: string | null;
	downloadName?: string;
	onPreview?: (attachment: PreviewTextAttachment) => void | Promise<void>;
	isPlaceholder?: boolean;
	isLoading?: boolean;
}> = ({
	content,
	fileName,
	mediaType,
	size,
	href,
	downloadName,
	onPreview,
	isPlaceholder,
	isLoading,
}) => {
	const displayName =
		fileName && fileName !== "Pasted text" ? fileName : "Pasted text";
	return (
		<FileAttachmentTile
			name={displayName}
			size={size}
			mediaType={mediaType}
			href={href}
			downloadName={downloadName}
			clickLabel={
				displayName === "Pasted text" ? "View text attachment" : undefined
			}
			isLoading={isLoading}
			preview={
				<span
					className={cn(
						"line-clamp-2 w-10 text-content-secondary",
						isPlaceholder ? "text-xs" : "font-mono text-2xs",
					)}
				>
					{isPlaceholder ? content : formatTextAttachmentPreview(content)}
				</span>
			}
			onClick={() => void onPreview?.({ content, fileName, mediaType })}
		/>
	);
};

const RemoteTextAttachmentButton: FC<{
	fileId: string;
	fileName?: string;
	mediaType?: string;
	size?: number;
	frameHref?: string | null;
	downloadName: string;
	onPreview?: (attachment: PreviewTextAttachment) => void | Promise<void>;
	showStatus?: boolean;
}> = ({
	fileId,
	fileName,
	mediaType,
	size,
	frameHref,
	downloadName,
	onPreview,
	showStatus = false,
}) => {
	const { hasExpired, markExpired } = useFileProbes();
	const isKnownExpired = hasExpired(fileId);
	const [content, setContent] = useState<string | null>(null);
	const [isLoading, setIsLoading] = useState(false);
	const [failureState, setFailureState] = useState<AttachmentFailureState>(
		() => (isKnownExpired ? { kind: "expired" } : { kind: "idle" }),
	);
	const request = useLatestAbortController(isKnownExpired);

	if (isKnownExpired) {
		return (
			<AttachmentFallbackTile
				state={{ kind: "expired" }}
				labels={textAttachmentFailureLabels}
				className="h-16 w-28"
			/>
		);
	}
	if (failureState.kind !== "idle") {
		return (
			<AttachmentFallbackTile
				state={failureState}
				labels={textAttachmentFailureLabels}
				className="h-16 w-28"
			/>
		);
	}

	const button = (
		<InlineTextAttachmentButton
			content={content ?? fileName ?? "Pasted text"}
			fileName={fileName}
			mediaType={mediaType}
			size={size}
			href={frameHref}
			downloadName={downloadName}
			isPlaceholder={content === null}
			isLoading={showStatus && isLoading}
			onPreview={async () => {
				if (isLoading) {
					return;
				}
				if (content !== null) {
					void onPreview?.({ content, fileName, mediaType });
					return;
				}

				const controller = request.start();
				setIsLoading(true);

				let result: Awaited<ReturnType<typeof fetchTextAttachmentContent>>;
				try {
					result = await fetchTextAttachmentContent(fileId, controller.signal);
				} catch (error) {
					if (!request.clear(controller)) {
						return;
					}
					setIsLoading(false);
					if (isAbortError(error)) {
						return;
					}
					console.warn("Failed to load text attachment:", error);
					setFailureState(attachmentFailureFromError(error));
					return;
				}

				if (!request.clear(controller)) {
					return;
				}
				setIsLoading(false);
				if (result.kind !== "loaded") {
					if (result.kind === "expired") {
						markExpired(fileId);
					}
					setFailureState(result);
					return;
				}
				setContent(result.content);
				void onPreview?.({ content: result.content, fileName, mediaType });
			}}
		/>
	);

	return (
		<div className="flex flex-col items-start gap-1">
			{button}
			{showStatus && isLoading ? (
				<span
					role="status"
					aria-live="polite"
					className="text-xs text-content-secondary"
				>
					Loading attachment preview
				</span>
			) : null}
		</div>
	);
};

const RemoteImageBlock: FC<{
	fileId?: string;
	href: string;
	displayName: string;
	downloadName: string;
	mediaType: string;
	size?: number;
	onImageClick?: (src: string) => void;
}> = ({
	fileId,
	href,
	displayName,
	downloadName,
	mediaType,
	size,
	onImageClick,
}) => {
	const {
		hasExpired,
		markExpired,
		isPending,
		markPending,
		clearPending,
		getProbeResult,
		setProbeResult,
	} = useFileProbes();
	const isKnownExpired = fileId !== undefined && hasExpired(fileId);
	const [failureState, setFailureState] = useState<AttachmentFailureState>(
		() => (isKnownExpired ? { kind: "expired" } : { kind: "idle" }),
	);
	const probeRequest = useLatestAbortController(isKnownExpired);

	if (isKnownExpired) {
		return (
			<AttachmentFallbackTile
				state={{ kind: "expired" }}
				labels={imageAttachmentFailureLabels}
			/>
		);
	}
	const sharedResult = fileId ? getProbeResult(fileId) : undefined;
	const effectiveFailure = sharedResult ?? failureState;
	if (effectiveFailure.kind !== "idle") {
		return (
			<AttachmentFallbackTile
				state={effectiveFailure}
				labels={imageAttachmentFailureLabels}
			/>
		);
	}

	return (
		<FileAttachmentTile
			name={displayName}
			size={size}
			mediaType={mediaType}
			href={href}
			downloadName={downloadName}
			preview={
				<ImageThumbnail
					previewUrl={href}
					name={displayName}
					className="h-10 w-10 cursor-pointer transition-opacity hover:opacity-80"
					onError={() => {
						if (fileId === undefined) {
							setFailureState({ kind: "failed" });
							return;
						}
						if (hasExpired(fileId)) {
							setFailureState({ kind: "expired" });
							return;
						}
						if (isPending(fileId)) {
							setFailureState({ kind: "failed" });
							return;
						}

						markPending(fileId);
						const controller = probeRequest.start();
						setFailureState({ kind: "failed" });

						void probeAttachmentFailure(href, controller.signal)
							.then((reason) => {
								clearPending(fileId);
								if (reason.kind === "expired") {
									markExpired(fileId);
								}
								setProbeResult(fileId, reason);
								if (probeRequest.clear(controller)) {
									setFailureState(reason);
								}
							})
							.catch((error) => {
								clearPending(fileId);
								if (isAbortError(error)) {
									return;
								}
								const failure = attachmentFailureFromError(error);
								setProbeResult(fileId, failure);
								if (probeRequest.clear(controller)) {
									setFailureState(failure);
								}
							});
					}}
				/>
			}
			onClick={() => onImageClick?.(href)}
		/>
	);
};

export const AttachmentBlock: FC<{
	block: FileAttachmentBlock;
	onImageClick?: (src: string) => void;
	onTextFileClick?: (attachment: PreviewTextAttachment) => void;
	framePreview?: boolean;
	showTextStatus?: boolean;
}> = ({
	block,
	onImageClick,
	onTextFileClick,
	framePreview = false,
	showTextStatus = false,
}) => {
	const [revealedInlineText, setRevealedInlineText] = useState(false);
	const href = getAttachmentHref(block);
	const displayName = getAttachmentDisplayName(block);
	const downloadName = getAttachmentDownloadName(block);

	if (isTextPreviewAttachmentMediaType(block.media_type)) {
		if (block.file_id) {
			return (
				<RemoteTextAttachmentButton
					fileId={block.file_id}
					fileName={displayName}
					mediaType={block.media_type}
					size={block.size}
					frameHref={framePreview ? href : undefined}
					downloadName={downloadName}
					onPreview={onTextFileClick}
					showStatus={showTextStatus}
				/>
			);
		}
		if (block.data == null) {
			return null;
		}
		const content = decodeInlineTextAttachment(block.data);
		return (
			<InlineTextAttachmentButton
				content={revealedInlineText ? content : "Pasted text"}
				fileName={displayName}
				mediaType={block.media_type}
				size={block.size}
				href={framePreview ? href : undefined}
				downloadName={downloadName}
				isPlaceholder={!revealedInlineText}
				onPreview={() => {
					setRevealedInlineText(true);
					void onTextFileClick?.({
						content,
						fileName: displayName,
						mediaType: block.media_type,
					});
				}}
			/>
		);
	}

	if (block.media_type.startsWith("image/")) {
		if (!href) {
			return null;
		}
		return (
			<RemoteImageBlock
				fileId={block.file_id ?? undefined}
				href={href}
				displayName={displayName}
				downloadName={downloadName}
				mediaType={block.media_type}
				size={block.size}
				onImageClick={onImageClick}
			/>
		);
	}

	if (!href) {
		return null;
	}

	return (
		<FileAttachmentTile
			name={displayName}
			size={block.size}
			mediaType={block.media_type}
			href={href}
			downloadName={downloadName}
		/>
	);
};
