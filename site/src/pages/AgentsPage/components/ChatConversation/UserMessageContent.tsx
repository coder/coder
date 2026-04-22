import { AlertTriangleIcon, FileTextIcon } from "lucide-react";
import { type FC, Fragment, useState } from "react";
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
import { Message, MessageContent } from "../ChatElements";
import { FileReferenceChip } from "../ChatMessageInput/FileReferenceNode";
import { useExpiredFileIds } from "./ExpiredFileIdsContext";
import type {
	MessageDisplayState,
	UserFileRenderBlock,
	UserInlineRenderBlock,
} from "./messageHelpers";

type ChatImageSource =
	| { kind: "file"; fileId: string; src: string }
	| { kind: "inline"; src: string };

type AttachmentFailureState = { kind: "idle" } | AttachmentFailure;

type AttachmentFailureLabels = {
	expired: string;
	failed: string;
};

const attachmentRetentionTooltip =
	"Chat attachments are deleted after the retention window set for this deployment.";

const imageAttachmentFailureLabels: AttachmentFailureLabels = {
	expired: "Image expired",
	failed: "Image failed to load",
};

const textAttachmentFailureLabels: AttachmentFailureLabels = {
	expired: "Attachment expired",
	failed: "Attachment failed to load",
};

const InlineTextAttachmentButton: FC<{
	content: string;
	onPreview?: (content: string) => void;
	isPlaceholder?: boolean;
}> = ({ content, onPreview, isPlaceholder }) => {
	return (
		<button
			type="button"
			aria-label="View text attachment"
			className="inline-flex h-16 max-w-sm items-center gap-2 rounded-md border-0 bg-surface-tertiary px-3 py-2 text-left transition-colors hover:bg-surface-quaternary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link"
			onClick={(event) => {
				event.stopPropagation();
				onPreview?.(content);
			}}
		>
			<FileTextIcon className="size-icon-sm shrink-0 text-content-secondary" />
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

const TextAttachmentButton: FC<{
	fileId: string;
	onPreview?: (content: string) => void;
}> = ({ fileId, onPreview }) => {
	const { hasExpired, markExpired } = useExpiredFileIds();
	const isKnownExpired = hasExpired(fileId);
	const [content, setContent] = useState<string | null>(null);
	const [failureState, setFailureState] = useState<AttachmentFailureState>(
		() => (isKnownExpired ? { kind: "expired" } : { kind: "idle" }),
	);
	const request = useLatestAbortController(isKnownExpired);

	if (failureState.kind === "expired" || isKnownExpired) {
		return (
			<AttachmentFallbackTile
				state={{ kind: "expired" }}
				labels={textAttachmentFailureLabels}
				className="h-16 w-28"
			/>
		);
	}
	if (failureState.kind === "failed") {
		return (
			<AttachmentFallbackTile
				state={failureState}
				labels={textAttachmentFailureLabels}
				className="h-16 w-28"
			/>
		);
	}

	return (
		<InlineTextAttachmentButton
			content={content ?? "Pasted text"}
			isPlaceholder={content === null}
			onPreview={() => {
				if (content !== null) {
					onPreview?.(content);
					return;
				}

				const controller = request.start();

				void fetchTextAttachmentContent(fileId, controller.signal)
					.then((result) => {
						if (!request.clear(controller)) {
							return;
						}
						if (result.kind === "loaded") {
							setContent(result.content);
							onPreview?.(result.content);
							return;
						}
						if (result.kind === "expired") {
							markExpired(fileId);
						}
						setFailureState(result);
					})
					.catch((error) => {
						if (!request.clear(controller)) {
							return;
						}
						if (isAbortError(error)) {
							return;
						}
						console.warn("Failed to load text attachment:", error);
						setFailureState(attachmentFailureFromError(error));
					});
			}}
		/>
	);
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
		state.kind === "expired" ? attachmentRetentionTooltip : state.detail;
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

const ChatImageBlock: FC<{
	source: ChatImageSource;
	onImageClick?: (src: string) => void;
}> = ({ source, onImageClick }) => {
	const { hasExpired, markExpired } = useExpiredFileIds();
	const isKnownExpired = source.kind === "file" && hasExpired(source.fileId);
	const [failureState, setFailureState] = useState<AttachmentFailureState>(
		() => (isKnownExpired ? { kind: "expired" } : { kind: "idle" }),
	);
	const probeRequest = useLatestAbortController(isKnownExpired);

	if (failureState.kind === "expired" || isKnownExpired) {
		return (
			<AttachmentFallbackTile
				state={{ kind: "expired" }}
				labels={imageAttachmentFailureLabels}
			/>
		);
	}
	if (failureState.kind === "failed") {
		return (
			<AttachmentFallbackTile
				state={failureState}
				labels={imageAttachmentFailureLabels}
			/>
		);
	}

	return (
		<button
			type="button"
			aria-label="View image"
			className="inline-block rounded-md border-0 bg-transparent p-0"
			onClick={(event) => {
				event.stopPropagation();
				onImageClick?.(source.src);
			}}
		>
			<ImageThumbnail
				previewUrl={source.src}
				name="Attached image"
				className="cursor-pointer transition-opacity hover:opacity-80"
				onError={() => {
					if (source.kind !== "file") {
						setFailureState({ kind: "failed" });
						return;
					}
					if (hasExpired(source.fileId)) {
						setFailureState({ kind: "expired" });
						return;
					}

					const controller = probeRequest.start();
					// Optimistically swap to the generic failure tile. The
					// probe will either upgrade it to "expired" or fill in
					// a detail; showing a tile without a label flash is
					// preferable to leaving the broken-image icon up.
					setFailureState({ kind: "failed" });

					void probeAttachmentFailure(source.src, controller.signal)
						.then((reason) => {
							if (!probeRequest.clear(controller)) {
								return;
							}
							if (reason.kind === "expired") {
								markExpired(source.fileId);
							}
							setFailureState(reason);
						})
						.catch((error) => {
							if (!probeRequest.clear(controller)) {
								return;
							}
							if (isAbortError(error)) {
								return;
							}
							setFailureState(attachmentFailureFromError(error));
						});
				}}
			/>
		</button>
	);
};

export const FileBlock: FC<{
	block: UserFileRenderBlock;
	onImageClick?: (src: string) => void;
	onTextFileClick?: (content: string) => void;
}> = ({ block, onImageClick, onTextFileClick }) => {
	if (block.media_type === "text/plain") {
		if (block.file_id) {
			return (
				<TextAttachmentButton
					fileId={block.file_id}
					onPreview={onTextFileClick}
				/>
			);
		}
		if (block.data != null) {
			return (
				<InlineTextAttachmentButton
					content={decodeInlineTextAttachment(block.data)}
					onPreview={onTextFileClick}
				/>
			);
		}
	}
	if (!block.media_type.startsWith("image/")) {
		return null;
	}
	const source: ChatImageSource = block.file_id
		? {
				kind: "file",
				fileId: block.file_id,
				src: getChatFileURL(block.file_id),
			}
		: {
				kind: "inline",
				src: `data:${block.media_type};base64,${block.data ?? ""}`,
			};
	return <ChatImageBlock source={source} onImageClick={onImageClick} />;
};

const renderUserInlineBlock = (block: UserInlineRenderBlock, index: number) => {
	if (block.type === "response") {
		return <Fragment key={index}>{block.text}</Fragment>;
	}

	return (
		<FileReferenceChip
			key={index}
			fileName={block.file_name}
			startLine={block.start_line}
			endLine={block.end_line}
			className="mx-1"
		/>
	);
};

export const UserMessageContent: FC<{
	displayState: MessageDisplayState;
	markdown: string;
	isEditing?: boolean;
	fadeFromBottom?: boolean;
	onImageClick?: (src: string) => void;
	onTextFileClick?: (content: string) => void;
}> = ({
	displayState,
	markdown,
	isEditing = false,
	fadeFromBottom = false,
	onImageClick,
	onTextFileClick,
}) => {
	return (
		<Message className="w-full max-w-none">
			<MessageContent
				className={cn(
					"rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm transition-shadow",
					isEditing &&
						"border-surface-secondary shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
					fadeFromBottom && "relative overflow-hidden",
				)}
				style={
					fadeFromBottom ? { maxHeight: "var(--clip-h, none)" } : undefined
				}
			>
				<div className="flex flex-col gap-1.5">
					{(displayState.hasUserMessageBody || displayState.hasFileBlocks) && (
						<div className="flex items-start gap-2">
							{displayState.hasUserMessageBody && (
								<span className="min-w-0 flex-1">
									{displayState.userInlineContent.length > 0
										? displayState.userInlineContent.map((block, index) =>
												renderUserInlineBlock(block, index),
											)
										: markdown || ""}
								</span>
							)}
						</div>
					)}
					{displayState.hasFileBlocks && (
						<div
							className={cn(
								displayState.hasUserMessageBody && "mt-2",
								"flex flex-wrap gap-2",
							)}
						>
							{displayState.userFileBlocks.map((block, index) => (
								<FileBlock
									key={`user-file-${block.file_id ?? index}`}
									block={block}
									onImageClick={onImageClick}
									onTextFileClick={onTextFileClick}
								/>
							))}
						</div>
					)}
					{fadeFromBottom && (
						<div
							className="pointer-events-none absolute inset-x-0 bottom-0 h-1/2 max-h-12"
							style={{
								opacity: "var(--fade-opacity, 0)",
								background:
									"linear-gradient(to top, hsl(var(--surface-secondary)), transparent)",
							}}
						/>
					)}
				</div>
			</MessageContent>
		</Message>
	);
};
