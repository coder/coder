import { cn } from "cn";
import { Fragment } from "react";
import { useWorkspaceFiles } from "../../context/WorkspaceFilesContext";
import { getSkillAlias } from "../../utils/skillAliases";
import {
	formatWorkspacePathReference,
	splitWorkspacePathReferences,
} from "../../utils/workspacePathReference";
import { Message, MessageContent } from "../ChatElements/Message";
import {
	FileReferenceChip,
	WorkspacePathChip,
} from "../ChatMessageInput/FileReferenceChip";
import {
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

// Paths that "@" chips sent render as chips again; files open on click.
const UserMessageText: React.FC<{ text: string }> = ({ text }) => {
	const workspaceFiles = useWorkspaceFiles();
	return (
		<>
			{splitWorkspacePathReferences(text).map((segment, index) => {
				if (segment.type === "text") {
					return <Fragment key={index}>{segment.text}</Fragment>;
				}
				const openFile =
					workspaceFiles && !segment.isDirectory
						? () => workspaceFiles.openFile(segment.path)
						: undefined;
				return (
					<span
						key={index}
						data-copy-text={formatWorkspacePathReference(
							segment.path,
							segment.isDirectory,
						)}
					>
						<WorkspacePathChip
							path={segment.path}
							isDirectory={segment.isDirectory}
							onOpen={openFile}
						/>
					</span>
				);
			})}
		</>
	);
};

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

const renderUserInlineBlock = (
	inlineParts: readonly InlinePart[],
	block: UserInlineRenderBlock,
	index: number,
) => {
	if (block.type === "response") {
		return <UserMessageText key={index} text={block.text} />;
	}
	const spacing = cn(
		hasInlineContentBefore(inlineParts, index) && "ml-1",
		hasInlineContentAfter(inlineParts, index) && "mr-1",
	);
	if (block.type === "skill-invocation") {
		return (
			<span
				key={index}
				data-copy-text={getSkillAlias(block.skill_source, block.skill_name)}
			>
				<SkillChip
					source={block.skill_source}
					name={block.skill_name}
					className={spacing}
				/>
			</span>
		);
	}

	return (
		<FileReferenceChip
			key={index}
			fileName={block.file_name}
			startLine={block.start_line}
			endLine={block.end_line}
			className={spacing}
		/>
	);
};

const renderUserInlineContent = (blocks: readonly UserInlineRenderBlock[]) => {
	const inlineParts = getInlineParts(blocks);
	return blocks.map((block, index) =>
		renderUserInlineBlock(inlineParts, block, index),
	);
};

export const UserMessageContent: React.FC<{
	displayState: MessageDisplayState;
	markdown: string;
	isEditing: boolean;
	onImageClick: (src: string) => void;
	onTextFileClick: (attachment: PreviewTextAttachment) => void;
}> = ({ displayState, markdown, isEditing, onImageClick, onTextFileClick }) => {
	return (
		<Message className="w-fit max-w-[min(80vw,80%)]">
			<MessageContent
				className={cn(
					"rounded-lg border border-solid border-border bg-surface-secondary px-3 py-2 font-sans shadow-xs transition-shadow",
					isEditing &&
						"border-surface-secondary shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
				)}
			>
				<div className="flex flex-col gap-1.5">
					{(displayState.hasUserMessageBody || displayState.hasFileBlocks) && (
						<div className="flex items-start gap-2">
							{displayState.hasUserMessageBody && (
								<span className="min-w-0 flex-1" onCopy={handleMessageCopy}>
									{displayState.userInlineContent.length > 0 ? (
										renderUserInlineContent(displayState.userInlineContent)
									) : (
										<UserMessageText text={markdown} />
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
								/>
							))}
						</div>
					)}
					{displayState.hasWorkspaceFileReferences && (
						<div
							className={cn(
								displayState.hasUserMessageBody && "mt-2",
								"flex flex-wrap gap-2",
							)}
						>
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
