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
	// Middle alignment sits a chip low in a body line it nearly fills and grows
	// the line. Headings keep middle alignment to center the chip on their text.
	const chipClassName = cn(
		"align-top in-[h1,h2,h3,h4,h5,h6]:align-middle",
		hasInlineContentBefore(inlineParts, index) && "ml-1",
		hasInlineContentAfter(inlineParts, index) && "mr-1",
	);
	if (block.type === "skill-invocation") {
		return {
			element: (
				<SkillChip
					source={block.skill_source}
					name={block.skill_name}
					className={chipClassName}
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
				className={chipClassName}
			/>
		),
		text: title,
	};
};

// At the bubble's top or bottom edge, the padding stands in for the
// half-leading of the 13px leading-relaxed message text, so files sit as far
// from the edge as text does.
const FileRow: React.FC<React.PropsWithChildren> = ({ children }) => (
	<div className="flex flex-wrap gap-2 first:pt-1 last:pb-1">{children}</div>
);

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
							className="whitespace-normal"
							preserveLineBreaks
							urlTransform={urlTransform}
						>
							{displayState.userInlineContent.length > 0
								? getUserMessageParts(displayState.userInlineContent)
								: markdown}
						</Response>
					)}
					{displayState.hasFileBlocks && (
						<FileRow>
							{displayState.userFileBlocks.map((block, index) => (
								<AttachmentBlock
									key={`user-file-${block.file_id ?? index}`}
									block={block}
									onImageClick={onImageClick}
									onTextFileClick={onTextFileClick}
								/>
							))}
						</FileRow>
					)}
					{displayState.hasWorkspaceFileReferences && (
						<FileRow>
							{displayState.workspaceFileBlocks.map((block, index) => (
								<WorkspaceFileChip
									key={`workspace-file-${block.workspace_file_path}-${index}`}
									name={block.workspace_file_name}
									path={block.workspace_file_path}
									size={block.workspace_file_size}
								/>
							))}
						</FileRow>
					)}
				</div>
			</MessageContent>
		</Message>
	);
};
