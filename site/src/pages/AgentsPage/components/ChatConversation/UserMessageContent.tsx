import { FileTextIcon } from "lucide-react";
import { type FC, Fragment, useEffect, useRef, useState } from "react";
import { cn } from "#/utils/cn";
import {
	decodeInlineTextAttachment,
	fetchTextAttachmentContent,
	formatTextAttachmentPreview,
} from "../../utils/fetchTextAttachment";
import { ImageThumbnail } from "../AgentChatInput";
import { Message, MessageContent } from "../ChatElements";
import { FileReferenceChip } from "../ChatMessageInput/FileReferenceNode";
import type {
	MessageDisplayState,
	UserFileRenderBlock,
	UserInlineRenderBlock,
} from "./messageHelpers";

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
	const [content, setContent] = useState<string | null>(null);
	const controllerRef = useRef<AbortController | null>(null);

	useEffect(() => {
		return () => controllerRef.current?.abort();
	}, []);

	return (
		<InlineTextAttachmentButton
			content={content ?? "Pasted text"}
			isPlaceholder={content === null}
			onPreview={async () => {
				if (content !== null) {
					onPreview?.(content);
					return;
				}

				controllerRef.current?.abort();
				const controller = new AbortController();
				controllerRef.current = controller;

				let fetchedContent: string;
				try {
					fetchedContent = await fetchTextAttachmentContent(
						fileId,
						controller.signal,
					);
				} catch (error) {
					if (controllerRef.current === controller) {
						controllerRef.current = null;
					}
					if (error instanceof Error && error.name === "AbortError") {
						return;
					}
					console.error("Failed to load text attachment:", error);
					return;
				}

				if (controllerRef.current === controller) {
					controllerRef.current = null;
				}
				setContent(fetchedContent);
				onPreview?.(fetchedContent);
			}}
		/>
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
	const src = block.file_id
		? `/api/experimental/chats/files/${block.file_id}`
		: `data:${block.media_type};base64,${block.data}`;
	return (
		<button
			type="button"
			aria-label="View image"
			className="inline-block rounded-md border-0 bg-transparent p-0"
			onClick={(event) => {
				event.stopPropagation();
				onImageClick?.(src);
			}}
		>
			<ImageThumbnail
				previewUrl={src}
				name="Attached image"
				className="cursor-pointer transition-opacity hover:opacity-80"
			/>
		</button>
	);
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
