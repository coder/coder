import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { chatDiffContents } from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import {
	ArrowLeftIcon,
	ExternalLinkIcon,
	GitBranchIcon,
	GitMergeIcon,
	GitPullRequestClosedIcon,
	GitPullRequestDraftIcon,
	GitPullRequestIcon,
} from "lucide-react";
import { type FC, type RefObject, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";
import { parsePullRequestUrl } from "../../utils/pullRequest";
import type { ChatMessageInputRef } from "../AgentChatInput";
import { CommentableDiffViewer } from "../DiffViewer/CommentableDiffViewer";
import { DiffStatBadge } from "../DiffViewer/DiffStats";
import type { DiffStyle } from "../DiffViewer/DiffViewer";

export { InlinePromptInput } from "../DiffViewer/CommentableDiffViewer";

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
	// Data fetching
	// ---------------------------------------------------------------
	const diffContentsQuery = useQuery({
		...chatDiffContents(chatId),
		enabled: Boolean(diffStatus?.url),
	});

	const diffContent = diffContentsQuery.data?.diff;
	const [diffVersion, setDiffVersion] = useState(0);
	const [prevDiffContent, setPrevDiffContent] = useState<string | undefined>(
		undefined,
	);
	if (diffContent !== prevDiffContent) {
		setPrevDiffContent(diffContent);
		setDiffVersion((v) => v + 1);
	}

	const parsedFiles = (() => {
		if (!diffContent) {
			return [] as FileDiffMetadata[];
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
				`chat-${chatId}-v${diffVersion}`,
			);
			return patches.flatMap((p) => p.files);
		} catch {
			return [] as FileDiffMetadata[];
		}
	})();

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

	const handleScrollComplete = () => {
		setScrollTarget(null);
	};

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
			<CommentableDiffViewer
				parsedFiles={parsedFiles}
				isExpanded={isExpanded}
				diffStyle={diffStyle}
				isLoading={diffContentsQuery.isLoading}
				error={diffContentsQuery.isError ? diffContentsQuery.error : undefined}
				chatInputRef={chatInputRef}
				scrollToFile={scrollTarget}
				onScrollToFileComplete={handleScrollComplete}
			/>
		</div>
	);
};
