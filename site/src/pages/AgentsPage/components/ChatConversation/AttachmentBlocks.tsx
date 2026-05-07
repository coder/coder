import {
	AlertTriangleIcon,
	DownloadIcon,
	FileIcon,
	FileTextIcon,
} from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { Spinner } from "#/components/Spinner/Spinner";
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

const getAttachmentBadgeLabel = (
	block: Pick<FileAttachmentBlock, "media_type" | "name">,
): string => {
	const extension = getAttachmentExtension(block);
	return extension === "file" ? "" : extension.toUpperCase();
};

const DownloadOverlay: FC<{
	href: string;
	displayName: string;
	downloadName: string;
}> = ({ href, displayName, downloadName }) => (
	<a
		href={href}
		download={downloadName}
		onClick={(event) => event.stopPropagation()}
		aria-label={`Download ${displayName}`}
		className="invisible absolute right-1 top-1 flex h-6 w-6 items-center justify-center rounded bg-surface-primary/80 text-content-secondary opacity-0 shadow-sm backdrop-blur-sm transition-opacity hover:text-content-primary group-hover/attachment:visible group-hover/attachment:opacity-100 group-focus-within/attachment:visible group-focus-within/attachment:opacity-100 [@media(hover:none)]:visible [@media(hover:none)]:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link"
	>
		<DownloadIcon aria-hidden="true" className="h-3.5 w-3.5" />
	</a>
);

const AttachmentPreviewFrame: FC<{
	href: string | null;
	displayName: string;
	downloadName: string;
	children: ReactNode;
}> = ({ href, displayName, downloadName, children }) => {
	return (
		<div className="group/attachment relative inline-flex flex-col items-start">
			{children}
			{href ? (
				<DownloadOverlay
					href={href}
					displayName={displayName}
					downloadName={downloadName}
				/>
			) : null}
		</div>
	);
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
	onPreview?: (attachment: PreviewTextAttachment) => void | Promise<void>;
	isPlaceholder?: boolean;
	icon?: ReactNode;
}> = ({ content, fileName, onPreview, isPlaceholder, icon }) => {
	return (
		<button
			type="button"
			aria-label={
				fileName && fileName !== "Pasted text"
					? `View ${fileName}`
					: "View text attachment"
			}
			className="inline-flex h-16 max-w-sm items-center gap-2 rounded-md border-0 bg-surface-tertiary px-3 py-2 text-left transition-colors hover:bg-surface-quaternary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link"
			onClick={(event) => {
				event.stopPropagation();
				void onPreview?.({ content, fileName });
			}}
		>
			{icon ?? (
				<FileTextIcon
					aria-hidden="true"
					className="size-icon-sm shrink-0 text-content-secondary"
				/>
			)}
			<span
				className={cn(
					"line-clamp-2 min-w-0 text-content-secondary",
					isPlaceholder ? "text-sm" : "font-mono text-xs",
				)}
			>
				{isPlaceholder ? content : formatTextAttachmentPreview(content)}
			</span>
		</button>
	);
};

const RemoteTextAttachmentButton: FC<{
	fileId: string;
	fileName?: string;
	mediaType?: string;
	frameHref?: string | null;
	downloadName: string;
	onPreview?: (attachment: PreviewTextAttachment) => void | Promise<void>;
	showStatus?: boolean;
}> = ({
	fileId,
	fileName,
	mediaType,
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
			icon={
				showStatus && isLoading ? (
					<Spinner
						size="sm"
						loading
						className="shrink-0 text-content-secondary"
					/>
				) : undefined
			}
			isPlaceholder={content === null}
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

	const framedButton = frameHref ? (
		<AttachmentPreviewFrame
			href={frameHref}
			displayName={fileName ?? "Pasted text"}
			downloadName={downloadName}
		>
			{button}
		</AttachmentPreviewFrame>
	) : (
		button
	);

	return (
		<div className="flex flex-col items-start gap-1">
			{framedButton}
			{showStatus && isLoading ? (
				<span
					role="status"
					aria-live="polite"
					className="text-xs text-content-secondary"
				>
					Loading attachment preview…
				</span>
			) : null}
		</div>
	);
};

const RemoteImageBlock: FC<{
	fileId?: string;
	href: string;
	displayName: string;
	onImageClick?: (src: string) => void;
}> = ({ fileId, href, displayName, onImageClick }) => {
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
		<button
			type="button"
			aria-label={`View ${displayName}`}
			className="inline-block rounded-md border-0 bg-transparent p-0"
			onClick={(event) => {
				event.stopPropagation();
				onImageClick?.(href);
			}}
		>
			<ImageThumbnail
				previewUrl={href}
				name={displayName}
				className="cursor-pointer transition-opacity hover:opacity-80"
				onError={() => {
					// Inline (data:) images can't be probed; the browser
					// already failed to decode them, so fall straight to a
					// generic failure tile.
					if (fileId === undefined) {
						setFailureState({ kind: "failed" });
						return;
					}
					if (hasExpired(fileId)) {
						setFailureState({ kind: "expired" });
						return;
					}
					// Dedup: skip probe, context will propagate the result.
					if (isPending(fileId)) {
						setFailureState({ kind: "failed" });
						return;
					}

					markPending(fileId);
					const controller = probeRequest.start();
					// Optimistically swap to the generic failure tile. The
					// probe will either upgrade it to "expired" or fill in
					// a detail; showing a tile without a label flash is
					// preferable to leaving the broken-image icon up.
					setFailureState({ kind: "failed" });

					void probeAttachmentFailure(href, controller.signal)
						.then((reason) => {
							clearPending(fileId);
							// Context writes stay above the clear() guard so
							// siblings get the result even if this block unmounted.
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
		</button>
	);
};

const FileCard: FC<{
	block: FileAttachmentBlock;
	href: string;
}> = ({ block, href }) => {
	const displayName = getAttachmentDisplayName(block);
	const downloadName = getAttachmentDownloadName(block);
	const badgeLabel = getAttachmentBadgeLabel(block);

	return (
		<a
			href={href}
			download={downloadName}
			onClick={(event) => event.stopPropagation()}
			aria-label={`Download ${displayName}`}
			className="inline-flex h-16 max-w-sm items-center gap-3 rounded-md border border-solid border-border-default bg-surface-tertiary px-3 py-2 no-underline transition-colors hover:bg-surface-quaternary"
		>
			<div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-surface-secondary">
				{badgeLabel ? (
					<span className="text-[10px] font-semibold tracking-wide text-content-secondary">
						{badgeLabel}
					</span>
				) : (
					<FileIcon
						aria-hidden="true"
						className="h-4 w-4 text-content-secondary"
					/>
				)}
			</div>
			<div className="min-w-0 flex-1">
				<div className="truncate text-sm text-content-primary">
					{displayName}
				</div>
				<div className="text-xs text-content-secondary">Download file</div>
			</div>
			<DownloadIcon
				aria-hidden="true"
				className="h-4 w-4 shrink-0 text-content-secondary"
			/>
		</a>
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
		const button = (
			<InlineTextAttachmentButton
				content={revealedInlineText ? content : "Pasted text"}
				fileName={displayName}
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
		return framePreview && href ? (
			<AttachmentPreviewFrame
				href={href}
				displayName={displayName}
				downloadName={downloadName}
			>
				{button}
			</AttachmentPreviewFrame>
		) : (
			button
		);
	}

	if (block.media_type.startsWith("image/")) {
		if (!href) {
			return null;
		}
		const image = (
			<RemoteImageBlock
				fileId={block.file_id ?? undefined}
				href={href}
				displayName={displayName}
				onImageClick={onImageClick}
			/>
		);
		return framePreview ? (
			<AttachmentPreviewFrame
				href={href}
				displayName={displayName}
				downloadName={downloadName}
			>
				{image}
			</AttachmentPreviewFrame>
		) : (
			image
		);
	}

	if (!href) {
		return null;
	}

	return <FileCard block={block} href={href} />;
};
