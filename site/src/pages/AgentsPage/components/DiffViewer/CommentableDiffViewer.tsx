import type {
	DiffLineAnnotation,
	FileDiffMetadata,
	SelectedLineRange,
} from "@pierre/diffs";
import { ArrowUpIcon } from "lucide-react";
import {
	type FC,
	type RefObject,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { Button } from "#/components/Button/Button";
import {
	annotationLineForBox,
	annotationSideForBox,
	type CommentBoxState,
	commentBoxFromRange,
	contentRangeForBox,
	selectedLinesForBox,
} from "../../utils/diffCommentSelection";
import type { ChatMessageInputRef } from "../AgentChatInput";
import type { DiffStyle } from "../DiffViewer/DiffViewer";
import { DiffViewer } from "../DiffViewer/DiffViewer";

// -------------------------------------------------------------------
// Diff content extraction
// -------------------------------------------------------------------

/**
 * Walk the parsed hunks for a file and collect code lines that fall
 * within `startLine..endLine` on the given side. For "additions"
 * lines are matched against addition line numbers (using
 * `hunk.additionStart`); for "deletions" against deletion line
 * numbers (using `hunk.deletionStart`). Context lines that fall
 * in range are included as well.
 */
export function extractDiffContent(
	parsedFiles: readonly FileDiffMetadata[],
	fileName: string,
	startLine: number,
	endLine: number,
	side: "additions" | "deletions",
): string {
	const file = parsedFiles.find((f) => f.name === fileName);
	if (!file) return "";

	const lines = side === "additions" ? file.additionLines : file.deletionLines;
	const collected: string[] = [];
	for (const hunk of file.hunks) {
		let addLine = hunk.additionStart;
		let delLine = hunk.deletionStart;

		for (const block of hunk.hunkContent) {
			if (block.type === "context") {
				for (let i = 0; i < block.lines; i++) {
					const ln = side === "additions" ? addLine : delLine;
					if (ln >= startLine && ln <= endLine) {
						const idx =
							side === "additions"
								? block.additionLineIndex + i
								: block.deletionLineIndex + i;
						if (lines[idx] != null) collected.push(lines[idx]);
					}
					addLine++;
					delLine++;
				}
			} else {
				// ChangeContent block.
				if (side === "deletions") {
					for (let i = 0; i < block.deletions; i++) {
						if (delLine >= startLine && delLine <= endLine) {
							const line = lines[block.deletionLineIndex + i];
							if (line != null) collected.push(line);
						}
						delLine++;
					}
					// Addition lines in a change block still advance
					// the addition counter.
					addLine += block.additions;
				} else {
					// side === "additions"
					// Deletion lines in a change block still advance
					// the deletion counter.
					delLine += block.deletions;
					for (let i = 0; i < block.additions; i++) {
						if (addLine >= startLine && addLine <= endLine) {
							const line = lines[block.additionLineIndex + i];
							if (line != null) collected.push(line);
						}
						addLine++;
					}
				}
			}
		}
	}

	return collected.join("\n");
}

// -------------------------------------------------------------------
// Inline prompt input
// -------------------------------------------------------------------

/**
 * Inline input rendered as a diff annotation under the selected
 * line(s). Supports multiline via Shift+Enter. Enter submits,
 * Escape dismisses.
 */
export const InlinePromptInput: FC<{
	onSubmit: (text: string) => void;
	onCancel: () => void;
}> = ({ onSubmit, onCancel }) => {
	const [text, setText] = useState("");
	const textareaRef = useRef<HTMLTextAreaElement>(null);

	useLayoutEffect(() => {
		textareaRef.current?.focus();
	}, []);

	return (
		<div className="px-2 py-1.5">
			<div className="rounded-lg border border-border-default/80 bg-surface-secondary/45 p-1 shadow-sm has-[textarea:focus]:ring-2 has-[textarea:focus]:ring-content-link/40">
				<textarea
					ref={textareaRef}
					className="w-full resize-none border-none bg-transparent px-3 py-2 font-sans text-sm leading-5 text-content-primary placeholder:text-content-secondary outline-none ring-0 focus:outline-none focus:ring-0"
					placeholder="Add a comment..."
					rows={2}
					value={text}
					onChange={(e) => setText(e.target.value)}
					onKeyDown={(e) => {
						if (e.key === "Enter" && !e.shiftKey) {
							e.preventDefault();
							if (text.trim()) {
								onSubmit(text.trim());
							} else {
								onCancel();
							}
						}
						if (e.key === "Escape") {
							e.preventDefault();
							onCancel();
						}
					}}
				/>
				<div className="flex items-end justify-between gap-2 pl-2.5 pr-1.5 pb-1.5">
					<span className="text-xs text-content-secondary">Esc to cancel</span>
					<Button
						size="icon"
						variant="default"
						className="size-7 rounded-full transition-colors [&>svg]:!size-4 [&>svg]:p-0"
						disabled={!text.trim()}
						onMouseDown={(e: React.MouseEvent) => {
							// Prevent blur from firing before click.
							e.preventDefault();
						}}
						onClick={() => {
							if (text.trim()) {
								onSubmit(text.trim());
							}
						}}
					>
						<ArrowUpIcon />
						<span className="sr-only">Add to chat</span>
					</Button>
				</div>
			</div>
		</div>
	);
};

// -------------------------------------------------------------------
// CommentableDiffViewer
// -------------------------------------------------------------------

interface CommentableDiffViewerProps {
	/** Parsed file diffs to render. */
	parsedFiles: readonly FileDiffMetadata[];
	/** Whether the panel is in expanded mode. */
	isExpanded?: boolean;
	/** Loading state. */
	isLoading?: boolean;
	/** Error state. */
	error?: unknown;
	/** Empty state message. */
	emptyMessage?: string;
	/** Which diff rendering style to use. */
	diffStyle: DiffStyle;
	/** Ref to the chat message input for inserting comments. */
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
	/** Scroll to a specific file. */
	scrollToFile?: string | null;
	/** Called after scrollToFile has been processed. */
	onScrollToFileComplete?: () => void;
}

/**
 * Wraps `DiffViewer` with inline commenting support. Click a line
 * number or select a range to open a comment input that inserts a
 * file reference chip and text into the chat input.
 */
export const CommentableDiffViewer: FC<CommentableDiffViewerProps> = ({
	parsedFiles,
	chatInputRef,
	...diffViewerProps
}) => {
	// ---------------------------------------------------------------
	// Comment / annotation state
	// ---------------------------------------------------------------
	const [activeCommentBox, setActiveCommentBox] =
		useState<CommentBoxState | null>(null);

	const activeCommentBoxRef = useRef<CommentBoxState | null>(null);

	const updateCommentBox = (box: CommentBoxState | null) => {
		activeCommentBoxRef.current = box;
		setActiveCommentBox(box);
	};

	// ---------------------------------------------------------------
	// Line interaction callbacks
	// ---------------------------------------------------------------
	const handleLineNumberClick = (
		fileName: string,
		props: {
			lineNumber: number;
			annotationSide: "additions" | "deletions";
		},
	) => {
		updateCommentBox({
			fileName,
			start: props.lineNumber,
			startSide: props.annotationSide,
			end: props.lineNumber,
			endSide: props.annotationSide,
		});
	};

	const handleLineSelected = (
		fileName: string,
		range: {
			start: number;
			end: number;
			side?: "additions" | "deletions";
			endSide?: "additions" | "deletions";
		} | null,
	) => {
		const result = commentBoxFromRange(fileName, range);
		if (result === "ignore") return;
		updateCommentBox(result);
	};

	// ---------------------------------------------------------------
	// Annotation helpers
	// ---------------------------------------------------------------
	const getLineAnnotations = (
		fileName: string,
	): DiffLineAnnotation<string>[] => {
		if (activeCommentBox && activeCommentBox.fileName === fileName) {
			return [
				{
					side: annotationSideForBox(activeCommentBox),
					lineNumber: annotationLineForBox(activeCommentBox),
					metadata: "active-input",
				},
			];
		}
		return [];
	};

	const getSelectedLines = (fileName: string): SelectedLineRange | null => {
		if (activeCommentBox && activeCommentBox.fileName === fileName) {
			return selectedLinesForBox(activeCommentBox);
		}
		return null;
	};

	const handleCancelComment = () => {
		updateCommentBox(null);
	};

	const handleSubmitComment = (text: string) => {
		const box = activeCommentBoxRef.current;
		if (!box) return;
		const { startLine, endLine, side } = contentRangeForBox(box);
		const content = extractDiffContent(
			parsedFiles,
			box.fileName,
			startLine,
			endLine,
			side,
		);
		// Single imperative call: chip inserted atomically
		// in one Lexical update. No rAF hack needed.
		chatInputRef?.current?.addFileReference({
			fileName: box.fileName,
			startLine,
			endLine,
			content,
		});
		if (text.trim()) {
			chatInputRef?.current?.insertText(text);
		}
		chatInputRef?.current?.focus();
		updateCommentBox(null);
	};

	const renderAnnotation = (annotation: DiffLineAnnotation<string>) => {
		if (annotation.metadata === "active-input") {
			return (
				<InlinePromptInput
					onSubmit={handleSubmitComment}
					onCancel={handleCancelComment}
				/>
			);
		}
		return null;
	};

	// ---------------------------------------------------------------
	// Render
	// ---------------------------------------------------------------
	return (
		<DiffViewer
			{...diffViewerProps}
			parsedFiles={parsedFiles}
			onLineNumberClick={handleLineNumberClick}
			onLineSelected={handleLineSelected}
			getLineAnnotations={getLineAnnotations}
			getSelectedLines={getSelectedLines}
			renderAnnotation={renderAnnotation}
		/>
	);
};
