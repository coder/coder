import { cn } from "cn";
import type { UrlTransform } from "streamdown";
import {
	useWorkspaceFiles,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import { getSkillAlias } from "../../utils/skillAliases";
import {
	formatWorkspacePathReference,
	splitWorkspacePathReferences,
} from "../../utils/workspacePathReference";
import type { InlineNode } from "../ChatElements/inlineNodes";
import { Message, MessageContent } from "../ChatElements/Message";
import { Response } from "../ChatElements/Response";
import {
	FileReferenceChip,
	WorkspacePathChip,
} from "../ChatMessageInput/FileReferenceChip";
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

// Middle alignment sits a chip low in a body line it nearly fills and grows
// the line. Headings keep middle alignment to center the chip on their text.
const chipAlignmentClassName = "align-top in-[h1,h2,h3,h4,h5,h6]:align-middle";

// Paths that "@" chips sent render as chips again. A file opens in a tab and
// a folder shows in the Files panel.
const splitWorkspacePathChips = (
	text: string,
	workspaceFiles: WorkspaceFilesContextValue | undefined,
): (string | InlineNode)[] =>
	splitWorkspacePathReferences(text).map((segment) => {
		if (segment.type === "text") {
			return segment.text;
		}
		const sentText = formatWorkspacePathReference(
			segment.path,
			segment.isDirectory,
		);
		let openPath: (() => void) | undefined;
		if (workspaceFiles && segment.isDirectory) {
			openPath = () =>
				workspaceFiles.revealInFiles(segment.path, { isDirectory: true });
		} else if (workspaceFiles) {
			openPath = () => workspaceFiles.openFile(segment.path);
		}
		return {
			element: (
				<span data-copy-text={sentText}>
					<WorkspacePathChip
						className={chipAlignmentClassName}
						path={segment.path}
						isDirectory={segment.isDirectory}
						onOpen={openPath}
						label={
							segment.isDirectory ? `Show ${segment.path} in Files` : undefined
						}
					/>
				</span>
			),
			text: sentText,
		};
	});

type CopySegment = { text: string; isChip: boolean };

// The text runs and chip copy texts of a copied fragment, in document order.
const getCopySegments = (node: Node): CopySegment[] => {
	if (node instanceof Element && node.hasAttribute("data-copy-text")) {
		return [{ text: node.getAttribute("data-copy-text") ?? "", isChip: true }];
	}
	if (node.nodeType === Node.TEXT_NODE) {
		return [{ text: node.textContent ?? "", isChip: false }];
	}
	return Array.from(node.childNodes).flatMap(getCopySegments);
};

// Chips are not selectable, so a copy of a selection within the message
// writes each chip's text form, its data-copy-text, in place of the chip.
// The server trims each sent text part, so one space separates a chip from
// text it would otherwise touch.
const handleMessageCopy = (event: React.ClipboardEvent<HTMLElement>) => {
	const selection = window.getSelection();
	const range = selection?.rangeCount ? selection.getRangeAt(0) : undefined;
	if (!range || !event.currentTarget.contains(range.commonAncestorContainer)) {
		return;
	}
	const fragment = range.cloneContents();
	if (!fragment.querySelector("[data-copy-text]")) {
		return;
	}
	let text = "";
	let previousIsChip = false;
	for (const segment of getCopySegments(fragment)) {
		if (!segment.text) {
			continue;
		}
		if (
			(segment.isChip || previousIsChip) &&
			/\S$/.test(text) &&
			/^\S/.test(segment.text)
		) {
			text += " ";
		}
		text += segment.text;
		previousIsChip = segment.isChip;
	}
	event.clipboardData.setData("text/plain", text);
	event.preventDefault();
};

const renderUserChip = (
	inlineParts: readonly InlinePart[],
	block: Exclude<UserInlineRenderBlock, { type: "response" }>,
	index: number,
): InlineNode => {
	const chipClassName = cn(
		chipAlignmentClassName,
		hasInlineContentBefore(inlineParts, index) && "ml-1",
		hasInlineContentAfter(inlineParts, index) && "mr-1",
	);
	if (block.type === "skill-invocation") {
		const alias = getSkillAlias(block.skill_source, block.skill_name);
		return {
			element: (
				<span data-copy-text={alias}>
					<SkillChip
						source={block.skill_source}
						name={block.skill_name}
						className={chipClassName}
					/>
				</span>
			),
			text: alias,
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

const getUserMessageParts = (
	blocks: readonly UserInlineRenderBlock[],
	workspaceFiles: WorkspaceFilesContextValue | undefined,
) => {
	const inlineParts = getInlineParts(blocks);
	return blocks.flatMap((block, index) =>
		block.type === "response"
			? splitWorkspacePathChips(block.text, workspaceFiles)
			: [renderUserChip(inlineParts, block, index)],
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
	const workspaceFiles = useWorkspaceFiles();
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
							onCopy={handleMessageCopy}
						>
							{displayState.userInlineContent.length > 0
								? getUserMessageParts(
										displayState.userInlineContent,
										workspaceFiles,
									)
								: splitWorkspacePathChips(markdown, workspaceFiles)}
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
