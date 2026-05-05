import { type FC, Fragment } from "react";
import { cn } from "#/utils/cn";
import { Message, MessageContent } from "../ChatElements";
import { FileReferenceChip } from "../ChatMessageInput/FileReferenceNode";
import {
	AttachmentBlock,
	type PreviewTextAttachment,
} from "./AttachmentBlocks";
import type {
	MessageDisplayState,
	UserInlineRenderBlock,
} from "./messageHelpers";

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
