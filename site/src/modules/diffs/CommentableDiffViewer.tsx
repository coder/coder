import type {
	DiffLineAnnotation,
	FileDiffMetadata,
	SelectedLineRange,
} from "@pierre/diffs";
import { ArrowUpIcon } from "lucide-react";
import {
	type FC,
	type MouseEvent,
	type ReactNode,
	type RefObject,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { Button } from "#/components/Button/Button";
import type { ChatMessageInputRef } from "#/pages/AgentsPage/components/AgentChatInput";
import {
	annotationLineForBox,
	annotationSideForBox,
	type CommentBoxState,
	commentBoxFromRange,
	contentRangeForBox,
	selectedLinesForBox,
} from "#/pages/AgentsPage/utils/diffCommentSelection";
import {
	ManagedDiffViewer,
	type ManagedDiffViewerProps,
} from "./ManagedDiffViewer";

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
	const file = parsedFiles.find((parsedFile) => parsedFile.name === fileName);
	if (!file) {
		return "";
	}

	const lines = side === "additions" ? file.additionLines : file.deletionLines;
	const collected: string[] = [];
	for (const hunk of file.hunks) {
		let addLine = hunk.additionStart;
		let delLine = hunk.deletionStart;

		for (const block of hunk.hunkContent) {
			if (block.type === "context") {
				for (let index = 0; index < block.lines; index++) {
					const lineNumber = side === "additions" ? addLine : delLine;
					if (lineNumber >= startLine && lineNumber <= endLine) {
						const lineIndex =
							side === "additions"
								? block.additionLineIndex + index
								: block.deletionLineIndex + index;
						if (lines[lineIndex] != null) {
							collected.push(lines[lineIndex]);
						}
					}
					addLine++;
					delLine++;
				}
			} else if (side === "deletions") {
				for (let index = 0; index < block.deletions; index++) {
					if (delLine >= startLine && delLine <= endLine) {
						const line = lines[block.deletionLineIndex + index];
						if (line != null) {
							collected.push(line);
						}
					}
					delLine++;
				}
				addLine += block.additions;
			} else {
				delLine += block.deletions;
				for (let index = 0; index < block.additions; index++) {
					if (addLine >= startLine && addLine <= endLine) {
						const line = lines[block.additionLineIndex + index];
						if (line != null) {
							collected.push(line);
						}
					}
					addLine++;
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
					onChange={(event) => setText(event.target.value)}
					onKeyDown={(event) => {
						if (event.key === "Enter" && !event.shiftKey) {
							event.preventDefault();
							if (text.trim()) {
								onSubmit(text.trim());
							} else {
								onCancel();
							}
						}
						if (event.key === "Escape") {
							event.preventDefault();
							onCancel();
						}
					}}
				/>
				<div className="flex items-end justify-between gap-2 pb-1.5 pl-2.5 pr-1.5">
					<span className="text-xs text-content-secondary">Esc to cancel</span>
					<Button
						size="icon"
						variant="default"
						className="size-7 rounded-full transition-colors [&>svg]:!size-4 [&>svg]:p-0"
						disabled={!text.trim()}
						onMouseDown={(event: MouseEvent<HTMLButtonElement>) => {
							// Prevent blur from firing before click.
							event.preventDefault();
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

interface CommentableDiffViewerProps
	extends Omit<
		ManagedDiffViewerProps,
		| "onLineNumberClick"
		| "onLineSelected"
		| "getLineAnnotations"
		| "getSelectedLines"
		| "renderAnnotation"
	> {
	/** Ref to the chat message input for inserting comments. */
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
}

interface CommentableCallbacks {
	handleLineNumberClick: (
		fileName: string,
		props: {
			lineNumber: number;
			annotationSide: "additions" | "deletions";
		},
	) => void;
	handleLineSelected: (
		fileName: string,
		range: {
			start: number;
			end: number;
			side?: "additions" | "deletions";
			endSide?: "additions" | "deletions";
		} | null,
	) => void;
	getLineAnnotations: (fileName: string) => DiffLineAnnotation<string>[];
	getSelectedLines: (fileName: string) => SelectedLineRange | null;
	renderAnnotation: (annotation: DiffLineAnnotation<string>) => ReactNode;
}

/**
 * Wraps `ManagedDiffViewer` with inline commenting support. Click a
 * line number or select a range to open a comment input that inserts
 * a file reference chip and text into the chat input.
 */
export const CommentableDiffViewer: FC<CommentableDiffViewerProps> = ({
	parsedFiles,
	chatInputRef,
	...diffViewerProps
}) => {
	const [activeCommentBox, setActiveCommentBox] =
		useState<CommentBoxState | null>(null);
	const activeCommentBoxRef = useRef<CommentBoxState | null>(activeCommentBox);
	const parsedFilesRef = useRef(parsedFiles);
	const chatInputRefRef = useRef(chatInputRef);
	const callbacksRef = useRef<CommentableCallbacks | null>(null);

	activeCommentBoxRef.current = activeCommentBox;
	parsedFilesRef.current = parsedFiles;
	chatInputRefRef.current = chatInputRef;

	let stableCallbacks = callbacksRef.current;
	if (!stableCallbacks) {
		const handleCancelComment = () => {
			setActiveCommentBox(null);
		};

		const handleSubmitComment = (text: string) => {
			const currentCommentBox = activeCommentBoxRef.current;
			if (!currentCommentBox) {
				return;
			}
			const { startLine, endLine, side } =
				contentRangeForBox(currentCommentBox);
			const content = extractDiffContent(
				parsedFilesRef.current,
				currentCommentBox.fileName,
				startLine,
				endLine,
				side,
			);
			const inputRef = chatInputRefRef.current?.current;
			// Single imperative call -- chip inserted atomically in one
			// Lexical update. No rAF hack needed.
			inputRef?.addFileReference({
				fileName: currentCommentBox.fileName,
				startLine,
				endLine,
				content,
			});
			if (text.trim()) {
				inputRef?.insertText(text);
			}
			inputRef?.focus();
			setActiveCommentBox(null);
		};

		stableCallbacks = {
			handleLineNumberClick: (fileName, props) => {
				setActiveCommentBox({
					fileName,
					start: props.lineNumber,
					startSide: props.annotationSide,
					end: props.lineNumber,
					endSide: props.annotationSide,
				});
			},
			handleLineSelected: (fileName, range) => {
				const result = commentBoxFromRange(fileName, range);
				if (result === "ignore") {
					return;
				}
				setActiveCommentBox(result);
			},
			getLineAnnotations: (fileName) => {
				const currentCommentBox = activeCommentBoxRef.current;
				if (currentCommentBox?.fileName !== fileName) {
					return [];
				}
				return [
					{
						side: annotationSideForBox(currentCommentBox),
						lineNumber: annotationLineForBox(currentCommentBox),
						metadata: "active-input",
					},
				];
			},
			getSelectedLines: (fileName) => {
				const currentCommentBox = activeCommentBoxRef.current;
				if (currentCommentBox?.fileName !== fileName) {
					return null;
				}
				return selectedLinesForBox(currentCommentBox);
			},
			renderAnnotation: (annotation) => {
				if (annotation.metadata !== "active-input") {
					return null;
				}
				if (!activeCommentBoxRef.current) {
					return null;
				}
				return (
					<InlinePromptInput
						onSubmit={handleSubmitComment}
						onCancel={handleCancelComment}
					/>
				);
			},
		};
		callbacksRef.current = stableCallbacks;
	}

	return (
		<ManagedDiffViewer
			{...diffViewerProps}
			parsedFiles={parsedFiles}
			onLineNumberClick={stableCallbacks.handleLineNumberClick}
			onLineSelected={stableCallbacks.handleLineSelected}
			getLineAnnotations={stableCallbacks.getLineAnnotations}
			getSelectedLines={stableCallbacks.getSelectedLines}
			renderAnnotation={stableCallbacks.renderAnnotation}
		/>
	);
};
