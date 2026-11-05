import { cn } from "cn";
import type { UrlTransform } from "streamdown";
import { getSkillAlias } from "../../utils/skillAliases";
import type { InlineNode } from "../ChatElements/inlineNodes";
import { Message, MessageContent } from "../ChatElements/Message";
import { Response } from "../ChatElements/Response";
import { FileReferenceChip } from "../ChatMessageInput/FileReferenceChip";
import {
	getFileReferenceDisplay,
	hasInlineContentAfter,
	hasInlineContentBefore,
	type InlinePart,
} from "../ChatMessageInput/fileReferenceDisplay";
import { SkillChip } from "../ChatMessageInput/SkillChip";
import {
	AttachmentBlock,
	type PreviewTextAttachment,
} from "./AttachmentBlocks";
import type {
	MessageDisplayState,
	UserInlineRenderBlock,
} from "./messageHelpers";
import { WorkspaceFileChip } from "./WorkspaceFileChip";

const getInlineParts = (
	blocks: readonly UserInlineRenderBlock[],
): InlinePart[] => {
	return blocks.map((block) => {
		if (block.type === "response") {
			return { type: "text", text: block.text };
		}
		return { type: "chip" };
	});
};

const renderUserChip = (
	inlineParts: readonly InlinePart[],
	block: Exclude<UserInlineRenderBlock, { type: "response" }>,
	index: number,
): InlineNode => {
	// A 20px chip nearly fills a 21px body line. Middle alignment centers it
	// on the x-height, which sits it 1.5px low and grows the line; top
	// alignment keeps the line height and the chip's drop shadow fills the
	// 1px left below it. Taller heading lines keep middle alignment.
	const spacing = cn(
		"align-top [:is(h1,h2,h3,h4,h5,h6)_&]:align-middle",
		hasInlineContentBefore(inlineParts, index) && "ml-1",
		hasInlineContentAfter(inlineParts, index) && "mr-1",
	);
	if (block.type === "skill-invocation") {
		return {
			element: (
				<SkillChip
					source={block.skill_source}
					name={block.skill_name}
					className={spacing}
				/>
			),
			text: getSkillAlias(block.skill_source, block.skill_name),
		};
	}

	const { title } = getFileReferenceDisplay({
		fileName: block.file_name,
		startLine: block.start_line,
		endLine: block.end_line,
	});
	return {
		element: (
			<FileReferenceChip
				fileName={block.file_name}
				startLine={block.start_line}
				endLine={block.end_line}
				className={spacing}
			/>
		),
		text: title,
	};
};

// Text lines carry about 4px of leading above and below their glyphs, so
// attachment rows at the bubble's top or bottom edge add the same amount to
// match the inset of text and of the horizontal padding.
const attachmentRowClassName = "flex flex-wrap gap-2 first:pt-1 last:pb-1";

const getUserMessageParts = (blocks: readonly UserInlineRenderBlock[]) => {
	const inlineParts = getInlineParts(blocks);
	return blocks.map((block, index) =>
		block.type === "response"
			? block.text
			: renderUserChip(inlineParts, block, index),
	);
};

export const UserMessageContent: React.FC<{
	displayState: MessageDisplayState;
	markdown: string;
	isEditing: boolean;
	onImageClick: (src: string, trigger: HTMLButtonElement) => void;
	onTextFileClick: (
		attachment: PreviewTextAttachment,
		trigger: HTMLButtonElement,
	) => void;
	urlTransform: UrlTransform;
}> = ({
	displayState,
	markdown,
	isEditing,
	onImageClick,
	onTextFileClick,
	urlTransform,
}) => {
	return (
		<Message className="w-fit max-w-[min(80vw,80%)]">
			<MessageContent
				className={cn(
					"rounded-lg border border-solid border-border bg-surface-secondary px-3 py-2 font-sans shadow-xs transition-shadow",
					isEditing &&
						"border-surface-secondary shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
				)}
			>
				<div className="flex flex-col gap-2">
					{displayState.hasUserMessageBody && (
						<Response
							className="min-w-0 whitespace-normal"
							preserveLineBreaks
							urlTransform={urlTransform}
						>
							{displayState.userInlineContent.length > 0
								? getUserMessageParts(displayState.userInlineContent)
								: markdown}
						</Response>
					)}
					{displayState.hasFileBlocks && (
						<div className={attachmentRowClassName}>
							{displayState.userFileBlocks.map((block, index) => (
								<AttachmentBlock
									key={`user-file-${block.file_id ?? index}`}
									block={block}
									onImageClick={onImageClick}
									onTextFileClick={onTextFileClick}
								/>
							))}
						</div>
					)}
					{displayState.hasWorkspaceFileReferences && (
						<div className={attachmentRowClassName}>
							{displayState.workspaceFileBlocks.map((block, index) => (
								<WorkspaceFileChip
									key={`workspace-file-${block.workspace_file_path}-${index}`}
									name={block.workspace_file_name}
									path={block.workspace_file_path}
									size={block.workspace_file_size}
								/>
							))}
						</div>
					)}
				</div>
			</MessageContent>
		</Message>
	);
};
