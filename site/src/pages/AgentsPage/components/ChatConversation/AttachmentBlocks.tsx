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
	handleAttachmentDownloadClick,
	isAbortError,
	isRasterImageMediaType,
} from "../../utils/chatAttachments";
import {
	decodeInlineTextAttachment,
	fetchTextAttachmentContent,
	formatTextAttachmentPreview,
} from "../../utils/fetchTextAttachment";
import { ImageThumbnail } from "../AgentChatInput";
import { useFileProbes } from "./FileProbeContext";
import type { RenderBlock } from "./types";
import {
	type AttachmentFailureState,
	useAttachmentFailure,
} from "./useAttachmentFailure";

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

/** Exact, case-sensitive match on the media type string. */
export const isTextPreviewAttachmentMediaType = (mediaType: string): boolean =>
	TEXT_ATTACHMENT_MEDIA_TYPES.has(mediaType);

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

type AttachmentDownload = {
	href: string;
	displayName: string;
	downloadName: string;
	mediaType: string;
};

const getAttachmentDownload = (
	block: FileAttachmentBlock,
	href: string,
): AttachmentDownload => ({
	href,
	displayName: getAttachmentDisplayName(block),
	downloadName: getAttachmentDownloadName(block),
	mediaType: block.media_type,
});

const AttachmentDownloadLink: React.FC<
	React.PropsWithChildren<
		AttachmentDownload & {
			className: string;
			label?: string;
			ref?: React.Ref<HTMLAnchorElement>;
		}
	>
> = ({
	href,
	displayName,
	downloadName,
	mediaType,
	className,
	label,
	ref,
	children,
}) => (
	<a
		ref={ref}
		href={href}
		download={downloadName}
		onClick={(event) => {
			event.stopPropagation();
			void handleAttachmentDownloadClick(event, {
				href,
				fileName: downloadName,
				mediaType,
			});
		}}
		aria-label={label ?? `Download ${displayName}`}
		className={className}
	>
		{children}
	</a>
);

const AttachmentPreviewFrame: React.FC<
	React.PropsWithChildren<AttachmentDownload>
> = ({ children, ...download }) => (
	<div className="group/attachment relative inline-flex flex-col items-start">
		{children}
		<AttachmentDownloadLink
			{...download}
			className="invisible absolute right-1 top-1 flex size-6 items-center justify-center rounded bg-surface-primary/80 text-content-secondary opacity-0 shadow-xs backdrop-blur-xs transition-opacity hover:text-content-primary group-hover/attachment:visible group-hover/attachment:opacity-100 group-focus-within/attachment:visible group-focus-within/attachment:opacity-100 [@media(hover:none)]:visible [@media(hover:none)]:opacity-100 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
		>
			<DownloadIcon aria-hidden="true" className="size-3.5" />
		</AttachmentDownloadLink>
	</div>
);

type AttachmentFailureLabels = {
	expired: string;
	failed: string;
};

const imageAttachmentFailureLabels: AttachmentFailureLabels = {
	expired: "Image expired",
	failed: "Image failed to load",
};

const fileAttachmentFailureLabels: AttachmentFailureLabels = {
	expired: "Attachment expired",
	failed: "Attachment failed to load",
};

const expiredAttachmentExplanation =
	"Attaching a file beyond a chat's attachment limit permanently deletes the chat's earliest-uploaded attachments. Attachments that no chat references are deleted after this deployment's retention window.";

const AttachmentFallbackTile: React.FC<{
	state: AttachmentFailure;
	labels: AttachmentFailureLabels;
	name?: string;
	className?: string;
	ref?: React.Ref<HTMLDivElement>;
}> = ({ state, labels, name, className = "size-16", ref }) => {
	const label = state.kind === "expired" ? labels.expired : labels.failed;
	// A bare "failed" (e.g. an inline base64 decode failure) has nothing to add.
	const tooltipBody =
		state.kind === "expired" ? expiredAttachmentExplanation : state.detail;

	const tile = (
		<div
			ref={ref}
			role="img"
			aria-label={name ? `${label}: ${name}` : label}
			// oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- needed to surface the explanation tooltip via keyboard focus.
			tabIndex={tooltipBody ? 0 : undefined}
			className={cn(
				"flex flex-col items-center justify-center gap-1 rounded-md border border-border bg-surface-tertiary px-1 text-center text-2xs text-content-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
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
	onPreview: (attachment: PreviewTextAttachment) => void | Promise<void>;
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
				void onPreview({ content, fileName });
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
	onPreview: (attachment: PreviewTextAttachment) => void | Promise<void>;
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
			onPreview={async () => {
				if (isLoading) {
					return;
				}
				if (content !== null) {
					void onPreview({ content, fileName, mediaType });
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
				void onPreview({ content: result.content, fileName, mediaType });
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
	onImageClick: (src: string) => void;
}> = ({ fileId, href, displayName, onImageClick }) => {
	const [failure, onError] = useAttachmentFailure(fileId, href);

	if (failure.kind !== "idle") {
		return (
			<AttachmentFallbackTile
				state={failure}
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
				onImageClick(href);
			}}
		>
			<ImageThumbnail
				previewUrl={href}
				name={displayName}
				className="cursor-pointer transition-opacity hover:opacity-80"
				onError={onError}
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

const FileCardDetails: React.FC<{
	tile: React.ReactNode;
	name: string;
	status: string;
}> = ({ tile, name, status }) => (
	<>
		<span className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-md bg-surface-secondary">
			{tile}
		</span>
		<span className="min-w-0 flex-1">
			<span className="block truncate text-sm text-content-primary">
				{name}
			</span>
			<span className="block text-xs text-content-secondary">{status}</span>
		</span>
	</>
);

/** Shows the file probes' expiry, but never probes the file itself. */
export const FileCard: React.FC<{
	block: FileAttachmentBlock;
	href: string;
	fullWidth?: boolean;
	/** Gets the download link, or the expired tile in its place. */
	ref?: React.RefCallback<HTMLElement>;
}> = ({ block, href, fullWidth = false, ref }) => {
	const { hasExpired } = useFileProbes();
	const download = getAttachmentDownload(block, href);

	if (block.file_id !== undefined && hasExpired(block.file_id)) {
		return (
			<AttachmentFallbackTile
				ref={ref}
				state={{ kind: "expired" }}
				labels={fileAttachmentFailureLabels}
				name={download.displayName}
				className={cn("h-16", fullWidth ? "w-full" : "w-28")}
			/>
		);
	}

	return (
		<AttachmentDownloadLink
			{...download}
			ref={ref}
			label={`${download.displayName} Download file`}
			className={cn(
				"inline-flex h-16 items-center gap-3 rounded-md border border-solid border-border bg-surface-tertiary px-3 py-2 no-underline transition-colors hover:bg-surface-quaternary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
				fullWidth ? "w-full" : "max-w-sm",
			)}
		>
			<FileCardDetails
				tile={<FileTypeTile block={block} />}
				name={download.displayName}
				status="Download file"
			/>
			<DownloadIcon
				aria-hidden="true"
				className="size-4 shrink-0 text-content-secondary"
			/>
		</AttachmentDownloadLink>
	);
};

/**
 * The visible name and status name the main button, so speech input can use
 * either.
 */
export const ViewableFileCard: React.FC<{
	block: FileAttachmentBlock;
	href: string;
	onView: (event: React.MouseEvent<HTMLButtonElement>) => void;
	/** Shown in place of the file type badge. */
	tile?: React.ReactNode;
	/** Shown under the name in place of "View file". */
	status?: string;
	ref?: React.RefCallback<HTMLElement>;
}> = ({ block, href, onView, tile, status = "View file", ref }) => {
	const download = getAttachmentDownload(block, href);

	return (
		<div
			ref={ref}
			className="inline-flex h-16 w-full items-center gap-1 rounded-md border border-solid border-border bg-surface-tertiary pr-2 transition-colors has-[>button:hover]:bg-surface-quaternary has-[>button:focus-visible]:ring-2 has-[>button:focus-visible]:ring-content-link"
		>
			<button
				type="button"
				aria-label={`${download.displayName} ${status}`}
				className="flex h-full min-w-0 flex-1 items-center gap-3 border-0 bg-transparent py-2 pl-3 text-left focus-visible:outline-hidden"
				onClick={onView}
			>
				<FileCardDetails
					tile={tile ?? <FileTypeTile block={block} />}
					name={download.displayName}
					status={status}
				/>
			</button>
			<AttachmentDownloadLink
				{...download}
				className="flex size-6 shrink-0 items-center justify-center rounded-md text-content-secondary transition-colors hover:bg-surface-secondary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
			>
				<DownloadIcon aria-hidden="true" className="size-4" />
			</AttachmentDownloadLink>
		</div>
	);
};

export const AttachmentBlock: React.FC<{
	block: FileAttachmentBlock;
	onImageClick: (src: string) => void;
	onTextFileClick: (attachment: PreviewTextAttachment) => void;
	framePreview?: boolean;
}> = ({ block, onImageClick, onTextFileClick, framePreview = false }) => {
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
					void onTextFileClick({
						content,
						fileName: displayName,
						mediaType: block.media_type,
					});
				}}
			/>
		);
		return framePreview && href ? (
			<AttachmentPreviewFrame {...getAttachmentDownload(block, href)}>
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
			<AttachmentPreviewFrame {...getAttachmentDownload(block, href)}>
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
