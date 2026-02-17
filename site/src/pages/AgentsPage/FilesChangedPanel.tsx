import { type FC, useMemo } from "react";
import { chatDiffContents, chatDiffStatus } from "api/queries/chats";
import { useQuery } from "react-query";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { parsePatchFiles } from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import {
	ExternalLinkIcon,
	GitBranchIcon,
	GitPullRequestIcon,
} from "lucide-react";

interface FilesChangedPanelProps {
	chatId: string;
}

/**
 * Extracts a short label like "owner/repo#123" from a GitHub PR URL.
 * Falls back to the raw URL if parsing fails.
 */
function formatPullRequestLabel(url: string): string {
	try {
		const match = url.match(
			/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)/,
		);
		if (match) {
			return `${match[1]}/${match[2]}#${match[3]}`;
		}
	} catch {
		// Fall through to return the raw URL.
	}
	return url;
}

export const FilesChangedPanel: FC<FilesChangedPanelProps> = ({ chatId }) => {
	const diffStatusQuery = useQuery(chatDiffStatus(chatId));
	const diffContentsQuery = useQuery({
		...chatDiffContents(chatId),
		enabled: Boolean(diffStatusQuery.data?.url),
	});

	const parsedFiles = useMemo(() => {
		const diff = diffContentsQuery.data?.diff;
		if (!diff) {
			return [];
		}
		const patches = parsePatchFiles(diff);
		return patches.flatMap((p) => p.files);
	}, [diffContentsQuery.data?.diff]);

	const pullRequestUrl = diffStatusQuery.data?.url;
	const pullRequestLabel = pullRequestUrl
		? formatPullRequestLabel(pullRequestUrl)
		: undefined;

	if (diffContentsQuery.isLoading || diffStatusQuery.isLoading) {
		return (
			<div className="flex h-full items-center justify-center">
				<Loader size="sm" />
			</div>
		);
	}

	if (diffContentsQuery.isError) {
		return (
			<div className="p-3">
				<ErrorAlert error={diffContentsQuery.error} />
			</div>
		);
	}

	return (
		<div className="flex h-full min-w-0 flex-col overflow-hidden border-0 border-l border-solid bg-surface-primary">
			{/* Header */}
			<div className="flex items-center justify-between gap-3 border-0 border-b border-l border-solid px-4 py-3">
				<div className="flex min-w-0 items-center gap-2">
					{pullRequestUrl ? (
						<>
							<GitPullRequestIcon className="h-4 w-4 shrink-0 text-content-secondary" />
							<span className="truncate text-sm font-medium text-content-primary">
								{pullRequestLabel}
							</span>
						</>
					) : (
						<>
							<GitBranchIcon className="h-4 w-4 text-content-secondary" />
							<span className="text-sm font-medium text-content-primary">
								Files Changed
							</span>
						</>
					)}
				</div>

				{pullRequestUrl && (
					<a
						href={pullRequestUrl}
						target="_blank"
						rel="noreferrer"
						className="flex shrink-0 items-center gap-1.5 rounded-md border border-border-default px-2.5 py-1 text-xs text-content-secondary no-underline transition-colors hover:bg-surface-tertiary hover:text-content-primary"
					>
						View PR
						<ExternalLinkIcon className="h-3 w-3" />
					</a>
				)}
			</div>

			{/* Diff contents */}
			{parsedFiles.length === 0 ? (
				<div className="flex flex-1 items-center justify-center p-6 text-center text-xs text-content-secondary">
					No file changes to display.
				</div>
			) : (
		<ScrollArea className="min-w-0 flex-1" scrollBarClassName="w-1.5">
			<div className="min-w-0 text-xs">
					{parsedFiles.map((fileDiff) => (
						<FileDiff
							key={fileDiff.name}
							fileDiff={fileDiff}
						options={{
							diffStyle: "unified",
							diffIndicators: "bars",
							overflow: "scroll",
							themeType: "dark",
							enableLineSelection: true,
							enableHoverUtility: true,
							theme: "github-dark-high-contrast",
						unsafeCSS:
							"pre, [data-line], [data-diffs-header] { background-color: transparent !important; } [data-diffs-header] { border-left: 1px solid var(--border); }",
						}}
						/>
					))}
				</div>
			</ScrollArea>
			)}
		</div>
	);
};

export default FilesChangedPanel;
