import { type FC, Fragment } from "react";
import { cn } from "#/utils/cn";
import { Message, MessageContent } from "../ChatElements";
import { FileReferenceChip } from "../ChatMessageInput/FileReferenceChip";
import {
	hasInlineContentAfter,
	hasInlineContentBefore,
	type InlinePart,
} from "../ChatMessageInput/fileReferenceDisplay";
import {
	AttachmentBlock,
	type PreviewTextAttachment,
} from "./AttachmentBlocks";
import type {
	MessageDisplayState,
	UserInlineRenderBlock,
} from "./messageHelpers";

const getInlineParts = (
	blocks: readonly UserInlineRenderBlock[],
): InlinePart[] => {
	return blocks.map((block) => {
		if (block.type === "file-reference") {
			return { type: "file-reference" };
		}
		return { type: "text", text: block.text };
	});
};

const renderUserInlineBlock = (
	inlineParts: readonly InlinePart[],
	block: UserInlineRenderBlock,
	index: number,
) => {
	if (block.type === "response") {
		return <Fragment key={index}>{block.text}</Fragment>;
	}

	return (
		<FileReferenceChip
			key={index}
			fileName={block.file_name}
			startLine={block.start_line}
			endLine={block.end_line}
			className={cn(
				hasInlineContentBefore(inlineParts, index) && "ml-1",
				hasInlineContentAfter(inlineParts, index) && "mr-1",
			)}
		/>
	);
};

const renderUserInlineContent = (blocks: readonly UserInlineRenderBlock[]) => {
	const inlineParts = getInlineParts(blocks);
	return blocks.map((block, index) =>
		renderUserInlineBlock(inlineParts, block, index),
	);
};

export const UserMessageContent: FC<{
	displayState: MessageDisplayState;
	markdown: string;
	isEditing?: boolean;
	fadeFromBottom?: boolean;
	onImageClick?: (src: string) => void;
	onTextFileClick?: (attachment: PreviewTextAttachment) => void;
}> = ({
	displayState,
	markdown,
	isEditing = false,
	fadeFromBottom = false,
	onImageClick,
	onTextFileClick,
}) => {
	return (
		<Message className="w-fit max-w-[min(80vw,80%)]">
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
										? renderUserInlineContent(displayState.userInlineContent)
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
								<AttachmentBlock
									key={`user-file-${block.file_id ?? index}`}
									block={block}
									onImageClick={onImageClick}
									onTextFileClick={onTextFileClick}
									showTextStatus
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
