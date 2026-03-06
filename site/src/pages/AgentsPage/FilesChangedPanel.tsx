import { parsePatchFiles } from "@pierre/diffs";
import { chatDiffContents, chatDiffStatus } from "api/queries/chats";
import {
	ExternalLinkIcon,
	GitBranchIcon,
	GitPullRequestIcon,
} from "lucide-react";
import { type FC, useMemo } from "react";
import { useQuery } from "react-query";
import { type DiffStyle, DiffViewer } from "./DiffViewer";

interface FilesChangedPanelProps {
	chatId: string;
	isExpanded?: boolean;
	diffStyle: DiffStyle;
}

/**
 * Parses a GitHub PR URL into its components.
 * Returns null if parsing fails.
 */
function parsePullRequestUrl(url: string): {
	owner: string;
	repo: string;
	number: string;
} | null {
	try {
		const match = url.match(/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)/);
		if (match) {
			return { owner: match[1], repo: match[2], number: match[3] };
		}
	} catch {
		// Fall through.
	}
	return null;
}

export const FilesChangedPanel: FC<FilesChangedPanelProps> = ({
	chatId,
	isExpanded,
	diffStyle,
}) => {
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
		try {
			// The cacheKeyPrefix enables the worker pool's LRU cache
			// so highlighted ASTs are reused across re-renders instead
			// of being re-computed on every render cycle.
			const patches = parsePatchFiles(diff, `chat-${chatId}`);
			return patches.flatMap((p) => p.files);
		} catch {
			return [];
		}
	}, [diffContentsQuery.data?.diff, chatId]);

	const pullRequestUrl = diffStatusQuery.data?.url;
	const parsedPr = pullRequestUrl ? parsePullRequestUrl(pullRequestUrl) : null;

	const headerLeft = pullRequestUrl ? (
		parsedPr ? (
			<a
				href={pullRequestUrl}
				target="_blank"
				rel="noreferrer"
				className="flex min-w-0 items-center gap-1.5 text-xs text-content-secondary no-underline hover:text-content-primary"
			>
				<GitPullRequestIcon className="h-3.5 w-3.5 shrink-0" />
				<span className="truncate">
					<span className="text-content-secondary">
						{parsedPr.owner}/{parsedPr.repo}
					</span>
					<span className="text-content-primary">#{parsedPr.number}</span>
				</span>
				<ExternalLinkIcon className="h-3 w-3 shrink-0 opacity-50" />
			</a>
		) : (
			<a
				href={pullRequestUrl}
				target="_blank"
				rel="noreferrer"
				className="flex min-w-0 items-center gap-1.5 text-xs text-content-secondary no-underline hover:text-content-primary"
			>
				<GitPullRequestIcon className="h-3.5 w-3.5 shrink-0" />
				<span className="truncate">{pullRequestUrl}</span>
				<ExternalLinkIcon className="h-3 w-3 shrink-0 opacity-50" />
			</a>
		)
	) : (
		<div className="flex items-center gap-1.5 text-xs text-content-secondary">
			<GitBranchIcon className="h-3.5 w-3.5" />
			<span>Uncommitted changes</span>
		</div>
	);

	return (
		<DiffViewer
			headerLeft={headerLeft}
			parsedFiles={parsedFiles}
			isExpanded={isExpanded}
			isLoading={diffContentsQuery.isLoading || diffStatusQuery.isLoading}
			error={diffContentsQuery.isError ? diffContentsQuery.error : undefined}
			diffStyle={diffStyle}
		/>
	);
};
