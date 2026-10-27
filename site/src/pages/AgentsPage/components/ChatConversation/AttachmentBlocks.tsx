import { cn } from "cn";
import {
	AlertTriangleIcon,
	DownloadIcon,
	FileIcon,
	FileTextIcon,
} from "lucide-react";
import { useState } from "react";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useLatestAbortController } from "../../hooks/useLatestAbortController";
import {
	type AttachmentFailure,
	attachmentFailureFromError,
	getAttachmentHref,
	isAbortError,
	isRasterImageMediaType,
	probeAttachmentFailure,
} from "../../utils/chatAttachments";
import {
	decodeInlineTextAttachment,
	fetchTextAttachmentContent,
	formatTextAttachmentPreview,
} from "../../utils/fetchTextAttachment";
import { ImageThumbnail } from "../AgentChatInput";
import {
	type AttachmentDownload,
	AttachmentDownloadLink,
} from "./AttachmentDownloadLink";
import { useFileProbes } from "./FileProbeContext";
import type { RenderBlock } from "./types";

export type PreviewTextAttachment = {
	content: string;
	fileName: string;
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
	"application/xml": "xml",
	"image/jpeg": "jpg",
	"image/svg+xml": "svg",
	"text/csv": "csv",
	"text/markdown": "md",
	"text/plain": "txt",
};

const sanitizeAttachmentExtension = (value: string): string =>
	value
		.replace(/[^a-z0-9]/gi, "")
		.slice(0, 4)
		.toLowerCase() || "file";

const getMediaTypeExtension = (mediaType: string): string | null => {
	if (mediaType === "application/octet-stream") {
		return null;
	}
	const mapped = ATTACHMENT_FALLBACK_EXTENSIONS[mediaType];
	if (mapped) {
		return mapped;
	}
	const [type, subtype = ""] = mediaType.split("/");
	// Unmapped non-image subtypes are not assumed to be filename extensions.
	return type === "image" && /^[a-z0-9]{1,8}$/i.test(subtype)
		? subtype.toLowerCase()
		: null;
};

const getNameExtension = (name: string): string | null => {
	const lastDot = name.lastIndexOf(".");
	return lastDot > 0 && lastDot < name.length - 1
		? name.slice(lastDot + 1)
		: null;
};

const getAttachmentExtension = (
	block: Pick<FileAttachmentBlock, "media_type" | "name">,
): string => {
	const mapped = ATTACHMENT_FALLBACK_EXTENSIONS[block.media_type];
	if (mapped) {
		return mapped;
	}
	const nameExtension = getNameExtension(block.name?.trim() ?? "");
	if (nameExtension) {
		return sanitizeAttachmentExtension(nameExtension);
	}
	return (
		getMediaTypeExtension(block.media_type) ??
		sanitizeAttachmentExtension(block.media_type.split("/")[1] ?? "")
	);
};

/** Plain text, Markdown, CSV, and JSON open in the text preview, whatever the file's name. */
export const isTextPreviewAttachmentMediaType = (mediaType: string): boolean =>
	TEXT_ATTACHMENT_MEDIA_TYPES.has(mediaType);

/** The file's name, or a label for its kind when it has none. */
export const getAttachmentDisplayName = (
	block: Pick<FileAttachmentBlock, "media_type" | "name">,
): string => {
	const name = block.name?.trim();
	if (name) {
		return name;
	}
	if (isRasterImageMediaType(block.media_type)) {
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
	if (!name) {
		const extension = getAttachmentExtension(block);
		return extension === "file" ? "attachment" : `attachment.${extension}`;
	}
	// Kept even when the name's extension disagrees with the media type.
	if (name.startsWith(".") || getNameExtension(name)) {
		return name;
	}
	const mediaExtension = getMediaTypeExtension(block.media_type);
	return mediaExtension ? `${name}.${mediaExtension}` : name;
};

const getAttachmentBadgeLabel = (
	block: Pick<FileAttachmentBlock, "media_type" | "name">,
): string => {
	const extension = getAttachmentExtension(block);
	return extension === "file" ? "" : extension.toUpperCase();
};

/** A nameless image shows as "Attached image" but saves as "attachment.png". */
export const getAttachmentDownload = (
	block: FileAttachmentBlock,
	href: string,
): AttachmentDownload => ({
	href,
	displayName: getAttachmentDisplayName(block),
	downloadName: getAttachmentDownloadName(block),
	mediaType: block.media_type,
});

const DownloadOverlay: React.FC<AttachmentDownload> = (download) => (
	<AttachmentDownloadLink
		{...download}
		// The frame sets the blur only while the link shows, so a hidden link adds
		// no compositing layer.
		className="absolute right-1 top-1 flex size-6 items-center justify-center rounded bg-surface-primary/80 text-content-secondary opacity-(--download-opacity) shadow-xs [backdrop-filter:var(--download-backdrop-filter)] transition-opacity [pointer-events:var(--download-pointer-events)] hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
	>
		<DownloadIcon aria-hidden="true" className="size-3.5" />
	</AttachmentDownloadLink>
);

const AttachmentPreviewFrame: React.FC<{
	href: string;
	displayName: string;
	downloadName: string;
	mediaType: string;
	children: React.ReactNode;
}> = ({ href, displayName, downloadName, mediaType, children }) => {
	return (
		// Not `group-focus-within`: WebKit restyles every ancestor's subtree for a
		// descendant `:focus-within` rule. The hidden link takes no pointer input but
		// stays tabbable, since focus shows it; `visibility` would make Shift+Tab skip it.
		<div className="relative inline-flex flex-col items-start [--download-opacity:0] [--download-pointer-events:none] [--download-backdrop-filter:none] hover:[--download-opacity:1] hover:[--download-pointer-events:auto] hover:[--download-backdrop-filter:blur(var(--blur-xs))] focus-within:[--download-opacity:1] focus-within:[--download-pointer-events:auto] focus-within:[--download-backdrop-filter:blur(var(--blur-xs))] [@media(hover:none)]:[--download-opacity:1] [@media(hover:none)]:[--download-pointer-events:auto] [@media(hover:none)]:[--download-backdrop-filter:blur(var(--blur-xs))]">
			{children}
			<DownloadOverlay
				href={href}
				displayName={displayName}
				downloadName={downloadName}
				mediaType={mediaType}
			/>
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

/** Status copy for a non-image file that is gone (`expired`) or would not load (`failed`). */
export const fileAttachmentFailureLabels: AttachmentFailureLabels = {
	expired: "Attachment expired",
	failed: "Attachment failed to load",
};

/**
 * Describes the eviction in coderd/database/queries/chats.sql and the
 * retention purge in coderd/database/dbpurge/dbpurge.go.
 */
export const expiredAttachmentExplanation =
	"Attaching a file beyond a chat's attachment limit permanently deletes the chat's earliest-uploaded attachments. When conversation retention is on, attachments that no chat references are deleted after the retention period.";

const AttachmentFallbackTile: React.FC<{
	state: AttachmentFailure;
	labels: AttachmentFailureLabels;
	className?: string;
}> = ({ state, labels, className = "size-16" }) => {
	const label = state.kind === "expired" ? labels.expired : labels.failed;

	const tile = (
		<div
			role="img"
			aria-label={label}
			className={cn(
				"flex flex-col items-center justify-center gap-1 rounded-md border border-border bg-surface-tertiary px-1 text-center text-2xs text-content-secondary",
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
	// - "expired" explains why the file is gone.
	// - "failed" with a detail surfaces the API error or network reason.
	// A bare "failed" (e.g. an inline base64 decode failure, where the
	// browser exposes nothing useful) stays a plain tile.
	const tooltipBody =
		state.kind === "expired" ? expiredAttachmentExplanation : state.detail;
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

const InlineTextAttachmentButton: React.FC<{
	content: string;
	fileName: string;
	onPreview: (trigger: HTMLButtonElement) => void | Promise<void>;
	isPlaceholder: boolean;
	icon?: React.ReactNode;
}> = ({ content, fileName, onPreview, isPlaceholder, icon }) => {
	return (
		<button
			type="button"
			aria-label={
				fileName !== "Pasted text" ? `View ${fileName}` : "View text attachment"
			}
			className="inline-flex h-16 max-w-sm items-center gap-2 rounded-md border-0 bg-surface-tertiary px-3 py-2 text-left transition-colors hover:bg-surface-quaternary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
			onClick={(event) => {
				event.stopPropagation();
				void onPreview(event.currentTarget);
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

const RemoteTextAttachmentButton: React.FC<{
	fileId: string;
	fileName: string;
	mediaType: string;
	frameHref?: string | null;
	downloadName: string;
	onPreview: (
		attachment: PreviewTextAttachment,
		trigger: HTMLButtonElement,
	) => void | Promise<void>;
}> = ({ fileId, fileName, mediaType, frameHref, downloadName, onPreview }) => {
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
				labels={fileAttachmentFailureLabels}
				className="h-16 w-28"
			/>
		);
	}
	if (failureState.kind !== "idle") {
		return (
			<AttachmentFallbackTile
				state={failureState}
				labels={fileAttachmentFailureLabels}
				className="h-16 w-28"
			/>
		);
	}

	const button = (
		<InlineTextAttachmentButton
			content={content ?? fileName}
			fileName={fileName}
			icon={
				isLoading ? (
					<Spinner
						size="sm"
						loading
						className="shrink-0 text-content-secondary"
					/>
				) : undefined
			}
			isPlaceholder={content === null}
			onPreview={async (trigger) => {
				if (isLoading) {
					return;
				}
				if (content !== null) {
					void onPreview({ content, fileName, mediaType }, trigger);
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
				void onPreview(
					{ content: result.content, fileName, mediaType },
					trigger,
				);
			}}
		/>
	);

	const framedButton = frameHref ? (
		<AttachmentPreviewFrame
			href={frameHref}
			displayName={fileName}
			downloadName={downloadName}
			mediaType={mediaType}
		>
			{button}
		</AttachmentPreviewFrame>
	) : (
		button
	);

	return (
		<div className="flex flex-col items-start gap-1">
			{framedButton}
			{isLoading ? (
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

const RemoteImageBlock: React.FC<{
	fileId?: string;
	href: string;
	displayName: string;
	onImageClick: (src: string, trigger: HTMLButtonElement) => void;
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
				onImageClick(href, event.currentTarget);
			}}
		>
			<ImageThumbnail
				previewUrl={href}
				alt={displayName}
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

const FileTypeTile: React.FC<{ block: FileAttachmentBlock }> = ({ block }) => {
	const badgeLabel = getAttachmentBadgeLabel(block);
	return badgeLabel ? (
		<span className="text-[10px] font-semibold tracking-wide text-content-secondary">
			{badgeLabel}
		</span>
	) : (
		<FileIcon aria-hidden="true" className="size-4 text-content-secondary" />
	);
};

/**
 * `tile` replaces the type badge, and `nameId` and `statusId` let a card's
 * control take its name from the visible text.
 */
export const FileCardDetails: React.FC<{
	block: FileAttachmentBlock;
	status: string;
	tile?: React.ReactNode;
	nameId?: string;
	statusId?: string;
}> = ({ block, status, tile, nameId, statusId }) => (
	<>
		<span className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-md bg-surface-secondary">
			{tile ?? <FileTypeTile block={block} />}
		</span>
		<span className="min-w-0 flex-1">
			<span id={nameId} className="block truncate text-sm text-content-primary">
				{getAttachmentDisplayName(block)}
			</span>
			<span id={statusId} className="block text-xs text-content-secondary">
				{status}
			</span>
		</span>
	</>
);

/**
 * Download card for a chat file, or the expired tile. Callers decide expiry;
 * the card never probes the file.
 */
export const FileCard: React.FC<{
	block: FileAttachmentBlock;
	href: string;
	expired: boolean;
	fullWidth?: boolean;
	ref?: React.RefCallback<HTMLElement>;
}> = ({ block, href, expired, fullWidth = false, ref }) => {
	if (expired) {
		return (
			<AttachmentFallbackTile
				state={{ kind: "expired" }}
				labels={fileAttachmentFailureLabels}
				className={cn("h-16", fullWidth ? "w-full" : "w-28")}
			/>
		);
	}

	return (
		<AttachmentDownloadLink
			{...getAttachmentDownload(block, href)}
			ref={ref}
			className={cn(
				"inline-flex h-16 items-center gap-3 rounded-md border border-solid border-border bg-surface-tertiary px-3 py-2 no-underline transition-colors hover:bg-surface-quaternary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
				fullWidth ? "w-full" : "max-w-sm",
			)}
		>
			<FileCardDetails block={block} status="Download file" />
			<DownloadIcon
				aria-hidden="true"
				className="size-4 shrink-0 text-content-secondary"
			/>
		</AttachmentDownloadLink>
	);
};

export const AttachmentBlock: React.FC<{
	block: FileAttachmentBlock;
	onImageClick: (src: string, trigger: HTMLButtonElement) => void;
	onTextFileClick: (
		attachment: PreviewTextAttachment,
		trigger: HTMLButtonElement,
	) => void;
	framePreview?: boolean;
}> = ({ block, onImageClick, onTextFileClick, framePreview = false }) => {
	const [revealedInlineText, setRevealedInlineText] = useState(false);
	const { hasExpired } = useFileProbes();
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
				onPreview={(trigger) => {
					setRevealedInlineText(true);
					void onTextFileClick(
						{ content, fileName: displayName, mediaType: block.media_type },
						trigger,
					);
				}}
			/>
		);
		return framePreview && href ? (
			<AttachmentPreviewFrame
				href={href}
				displayName={displayName}
				downloadName={downloadName}
				mediaType={block.media_type}
			>
				{button}
			</AttachmentPreviewFrame>
		) : (
			button
		);
	}

	if (isRasterImageMediaType(block.media_type)) {
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
				mediaType={block.media_type}
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

	return (
		<FileCard
			block={block}
			href={href}
			expired={block.file_id !== undefined && hasExpired(block.file_id)}
		/>
	);
};
