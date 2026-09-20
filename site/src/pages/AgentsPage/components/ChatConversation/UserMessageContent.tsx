import { cn } from "cn";
import { type FC, Fragment } from "react";
import type { ChatSenderChatPart } from "#/api/typesGenerated";
import { Message, MessageContent } from "../ChatElements/Message";
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
import { SenderChatHeader } from "./SenderChatHeader";

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
	/** Present when another chat's agent delivered this prompt. */
	senderChat?: ChatSenderChatPart;
	onImageClick?: (src: string) => void;
	onTextFileClick?: (attachment: PreviewTextAttachment) => void;
}> = ({
	displayState,
	markdown,
	isEditing = false,
	senderChat,
	onImageClick,
	onTextFileClick,
}) => {
	return (
		<Message className="w-fit max-w-[min(80vw,80%)]">
			<MessageContent
				className={cn(
					"rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-xs transition-shadow",
					senderChat && "border-l-2 border-l-content-link",
					isEditing &&
						"border-surface-secondary shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
				)}
			>
				<div className="flex flex-col gap-1.5">
					{senderChat && <SenderChatHeader senderChat={senderChat} />}
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
				</div>
			</MessageContent>
		</Message>
	);
};
