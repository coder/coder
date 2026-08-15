import type { FC } from "react";
import type { UrlTransform } from "streamdown";
import { cn } from "#/utils/cn";
import { Message, MessageContent } from "../ChatElements";
import { LinkifiedText } from "../ChatElements/LinkifiedText";
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
	urlTransform?: UrlTransform,
) => {
	if (block.type === "response") {
		return (
			<LinkifiedText
				key={index}
				text={block.text}
				urlTransform={urlTransform}
			/>
		);
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

const renderUserInlineContent = (
	blocks: readonly UserInlineRenderBlock[],
	urlTransform?: UrlTransform,
) => {
	const inlineParts = getInlineParts(blocks);
	return blocks.map((block, index) =>
		renderUserInlineBlock(inlineParts, block, index, urlTransform),
	);
};

export const UserMessageContent: FC<{
	displayState: MessageDisplayState;
	markdown: string;
	urlTransform?: UrlTransform;
	isEditing?: boolean;
	onImageClick?: (src: string) => void;
	onTextFileClick?: (attachment: PreviewTextAttachment) => void;
}> = ({
	displayState,
	markdown,
	urlTransform,
	isEditing = false,
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
				)}
			>
				<div className="flex flex-col gap-1.5">
					{(displayState.hasUserMessageBody || displayState.hasFileBlocks) && (
						<div className="flex items-start gap-2">
							{displayState.hasUserMessageBody && (
								<span className="min-w-0 flex-1">
									{displayState.userInlineContent.length > 0
										? renderUserInlineContent(
												displayState.userInlineContent,
												urlTransform,
											)
										: markdown && (
												<LinkifiedText
													text={markdown}
													urlTransform={urlTransform}
												/>
											)}
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
