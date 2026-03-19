import type {
	DiffLineAnnotation,
	FileDiffMetadata,
	SelectedLineRange,
} from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { chatDiffContents } from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	ArrowLeftIcon,
	ArrowUpIcon,
	ExternalLinkIcon,
	GitBranchIcon,
	GitMergeIcon,
	GitPullRequestClosedIcon,
	GitPullRequestDraftIcon,
	GitPullRequestIcon,
} from "lucide-react";
import {
	type FC,
	type RefObject,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";
import type { ChatMessageInputRef } from "./AgentChatInput";
import { DiffStatBadge } from "./DiffStats";
import type { DiffStyle } from "./DiffViewer";
import { DiffViewer } from "./DiffViewer";
import { parsePullRequestUrl } from "./pullRequest";

// -------------------------------------------------------------------
// Module-level counter for cache key uniqueness
// -------------------------------------------------------------------

/**
 * Monotonic counter shared across all RemoteDiffPanel instances.
 * Ensures parsePatchFiles cache keys never collide across mounts,
 * since component-local refs reset to 0 on remount while the
 * worker pool's LRU cache persists.
 */
let remoteDiffVersion = 0;

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
function extractDiffContent(
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
// PR state badge
// -------------------------------------------------------------------

const PullRequestStateBadge: FC<{
	state?: string;
	draft?: boolean;
}> = ({ state, draft }) => {
	let Icon = GitPullRequestIcon;
	let label = "Open";
	let colorClasses = "bg-surface-git-added text-git-added-bright";

	if (state === "merged") {
		Icon = GitMergeIcon;
		label = "Merged";
		colorClasses = "bg-surface-git-merged text-git-merged-bright";
	} else if (state === "closed") {
		Icon = GitPullRequestClosedIcon;
		label = "Closed";
		colorClasses = "bg-surface-git-deleted text-git-deleted-bright";
	} else if (draft) {
		Icon = GitPullRequestDraftIcon;
		label = "Draft";
		colorClasses = "text-content-secondary";
	}

	return (
		<span
			className={cn(
				"inline-flex shrink-0 items-center gap-1 rounded-sm border border-solid border-border-default px-2 text-[13px] font-medium leading-5",
				colorClasses,
			)}
		>
			<Icon className="size-3" />
			{label}
		</span>
	);
};

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

	// Focus the textarea on mount. We use a ref callback via rAF
	// rather than autoFocus because the component renders inside
	// Shadow DOM where autoFocus is unreliable.
	useEffect(() => {
		requestAnimationFrame(() => {
			textareaRef.current?.focus();
		});
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
// Main component
// -------------------------------------------------------------------

interface RemoteDiffPanelProps {
	chatId: string;
	isExpanded?: boolean;
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
	diffStyle: DiffStyle;
	diffStatus?: TypesGen.ChatDiffStatus;
}

export const RemoteDiffPanel: FC<RemoteDiffPanelProps> = ({
	chatId,
	isExpanded,
	chatInputRef,
	diffStyle,
	diffStatus,
}) => {
	// ---------------------------------------------------------------
	// Comment / annotation state
	// ---------------------------------------------------------------
	const [activeCommentBox, setActiveCommentBox] = useState<{
		fileName: string;
		startLine: number;
		endLine: number;
		side: "additions" | "deletions";
	} | null>(null);

	// ---------------------------------------------------------------
	// Data fetching
	// ---------------------------------------------------------------
	const diffContentsQuery = useQuery({
		...chatDiffContents(chatId),
		enabled: Boolean(diffStatus?.url),
	});

	const diffContent = diffContentsQuery.data?.diff;
	const diffVersionRef = useRef(0);
	const prevDiffRef = useRef<string | undefined>(undefined);
	if (diffContent !== prevDiffRef.current) {
		prevDiffRef.current = diffContent;
		diffVersionRef.current = ++remoteDiffVersion;
	}

	const parsedFiles = useMemo(() => {
		if (!diffContent) {
			return [];
		}
		try {
			// The cacheKeyPrefix enables the worker pool's LRU cache
			// so highlighted ASTs are reused across re-renders instead
			// of being re-computed on every render cycle. We include a
			// version counter derived from the diff content so that when
			// the diff changes (e.g. new commits pushed) the old cached
			// highlight AST is not reused with mismatched line indices,
			// which would cause DiffHunksRenderer.processDiffResult to
			// throw. Unlike dataUpdatedAt, this counter only increments
			// when the actual diff string changes, avoiding unnecessary
			// recomputation on refetches with identical content.
			const patches = parsePatchFiles(
				diffContent,
				`chat-${chatId}-v${diffVersionRef.current}`,
			);
			return patches.flatMap((p) => p.files);
		} catch {
			return [];
		}
	}, [diffContent, chatId]);

	// ---------------------------------------------------------------
	// Line interaction callbacks
	// ---------------------------------------------------------------
	const handleLineNumberClick = useCallback(
		(
			fileName: string,
			props: {
				lineNumber: number;
				annotationSide: "additions" | "deletions";
			},
		) => {
			setActiveCommentBox({
				fileName,
				startLine: props.lineNumber,
				endLine: props.lineNumber,
				side: props.annotationSide,
			});
		},
		[],
	);

	const handleLineSelected = useCallback(
		(
			fileName: string,
			range: {
				start: number;
				end: number;
				side?: "additions" | "deletions";
			} | null,
		) => {
			if (!range) {
				setActiveCommentBox(null);
				return;
			}
			if (range.start === range.end) return;
			const side = range.side ?? "additions";
			setActiveCommentBox({
				fileName,
				startLine: Math.min(range.start, range.end),
				endLine: Math.max(range.start, range.end),
				side,
			});
		},
		[],
	);

	// ---------------------------------------------------------------
	// Annotation helpers
	// ---------------------------------------------------------------
	const getLineAnnotations = useCallback(
		(fileName: string): DiffLineAnnotation<string>[] => {
			if (activeCommentBox && activeCommentBox.fileName === fileName) {
				return [
					{
						side: activeCommentBox.side,
						lineNumber: activeCommentBox.endLine,
						metadata: "active-input",
					},
				];
			}
			return [];
		},
		[activeCommentBox],
	);

	const getSelectedLines = useCallback(
		(fileName: string): SelectedLineRange | null => {
			if (activeCommentBox && activeCommentBox.fileName === fileName) {
				return {
					start: activeCommentBox.startLine,
					end: activeCommentBox.endLine,
					side: activeCommentBox.side,
				};
			}
			return null;
		},
		[activeCommentBox],
	);

	const handleCancelComment = useCallback(() => {
		setActiveCommentBox(null);
	}, []);

	const handleSubmitComment = useCallback(
		(text: string) => {
			if (!activeCommentBox) return;
			const content = extractDiffContent(
				parsedFiles,
				activeCommentBox.fileName,
				activeCommentBox.startLine,
				activeCommentBox.endLine,
				activeCommentBox.side,
			);
			// Single imperative call — chip inserted atomically
			// in one Lexical update. No rAF hack needed.
			chatInputRef?.current?.addFileReference({
				fileName: activeCommentBox.fileName,
				startLine: activeCommentBox.startLine,
				endLine: activeCommentBox.endLine,
				content,
			});
			if (text.trim()) {
				chatInputRef?.current?.insertText(text);
			}
			setActiveCommentBox(null);
		},
		[activeCommentBox, chatInputRef, parsedFiles],
	);

	const renderAnnotation = useCallback(
		(annotation: DiffLineAnnotation<string>) => {
			if (annotation.metadata === "active-input") {
				if (!activeCommentBox) return null;
				return (
					<InlinePromptInput
						onSubmit={handleSubmitComment}
						onCancel={handleCancelComment}
					/>
				);
			}
			return null;
		},
		[activeCommentBox, handleSubmitComment, handleCancelComment],
	);

	// ---------------------------------------------------------------
	// Scroll-to-file from chat input chip clicks
	// ---------------------------------------------------------------
	const [scrollTarget, setScrollTarget] = useState<string | null>(null);

	useEffect(() => {
		const handler = (e: Event) => {
			const { fileName } = (e as CustomEvent).detail ?? {};
			if (typeof fileName !== "string") return;
			setScrollTarget(fileName);
		};
		window.addEventListener("file-reference-click", handler);
		return () => window.removeEventListener("file-reference-click", handler);
	}, []);

	const handleScrollComplete = useCallback(() => {
		setScrollTarget(null);
	}, []);

	// ---------------------------------------------------------------
	// Header content
	// ---------------------------------------------------------------
	const pullRequestUrl = diffStatus?.url;
	const parsedPr = pullRequestUrl ? parsePullRequestUrl(pullRequestUrl) : null;
	const prState = diffStatus?.pull_request_state;
	const prDraft = diffStatus?.pull_request_draft;
	const baseBranch = diffStatus?.base_branch;
	const headBranch = diffStatus?.head_branch;

	// ---------------------------------------------------------------
	// Render
	// ---------------------------------------------------------------
	return (
		<div className="flex h-full flex-col">
			{/* Compact PR sub-header */}
			{pullRequestUrl && (
				<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3 py-1.5">
					<div className="flex min-w-0 items-center gap-1.5 text-[13px] text-content-secondary">
						{baseBranch || headBranch ? (
							<>
								<GitBranchIcon className="size-3.5 shrink-0" />
								{baseBranch && <span className="truncate">{baseBranch}</span>}
								{headBranch && baseBranch && (
									<ArrowLeftIcon className="size-3 shrink-0 opacity-50" />
								)}
								{headBranch && <span className="truncate"> {headBranch}</span>}
							</>
						) : parsedPr ? (
							<span className="truncate">
								{parsedPr.owner}/{parsedPr.repo}#{parsedPr.number}
							</span>
						) : (
							<span className="truncate">{pullRequestUrl}</span>
						)}
					</div>
					<div className="ml-auto flex shrink-0 items-center gap-1.5">
						<PullRequestStateBadge state={prState} draft={prDraft} />
						{diffStatus?.additions || diffStatus?.deletions ? (
							<DiffStatBadge
								additions={diffStatus.additions}
								deletions={diffStatus.deletions}
							/>
						) : null}
						<a
							href={pullRequestUrl}
							target="_blank"
							rel="noreferrer"
							className="inline-flex items-center gap-1 rounded-sm border border-solid border-border-default px-2 text-[13px] font-medium leading-5 text-content-secondary no-underline transition-colors hover:bg-surface-secondary hover:text-content-primary"
						>
							View PR
							<ExternalLinkIcon className="size-3" />
						</a>
					</div>
				</div>
			)}
			<DiffViewer
				parsedFiles={parsedFiles}
				isExpanded={isExpanded}
				diffStyle={diffStyle}
				isLoading={diffContentsQuery.isLoading}
				error={diffContentsQuery.isError ? diffContentsQuery.error : undefined}
				onLineNumberClick={handleLineNumberClick}
				onLineSelected={handleLineSelected}
				getLineAnnotations={getLineAnnotations}
				getSelectedLines={getSelectedLines}
				renderAnnotation={renderAnnotation}
				scrollToFile={scrollTarget}
				onScrollToFileComplete={handleScrollComplete}
			/>
		</div>
	);
};
