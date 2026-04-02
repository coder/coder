import type { FC } from "react";
import type { ChatDiffStatus } from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import { parsePullRequestUrl } from "../../utils/pullRequest";
import { PrStateIcon } from "../GitPanel/GitPanel";

interface PullRequestBadgeProps {
	diffStatusData: ChatDiffStatus;
	hiddenWhenPanelOpen?: boolean;
}

export const PullRequestBadge: FC<PullRequestBadgeProps> = ({
	diffStatusData,
	hiddenWhenPanelOpen,
}) => {
	const prUrl = diffStatusData.url;
	const prState = diffStatusData.pull_request_state;
	const prDraft = diffStatusData.pull_request_draft;
	const prTitle = diffStatusData.pull_request_title;
	const parsedPr = parsePullRequestUrl(prUrl);
	const prNumberMatch =
		diffStatusData.pr_number?.toString() ?? parsedPr?.number;
	const hasPR = Boolean(prState || prNumberMatch || parsedPr);

	if (!prUrl || !hasPR) {
		return null;
	}

	return (
		<a
			href={prUrl}
			target="_blank"
			rel="noreferrer"
			className={cn(
				"inline-flex shrink-0 items-center gap-1.5 rounded-md border border-solid border-border-default px-2 py-0.5 text-xs font-medium text-content-secondary no-underline transition-colors hover:bg-surface-secondary hover:text-content-primary",
				hiddenWhenPanelOpen && "md:hidden",
			)}
		>
			<PrStateIcon
				state={prState}
				draft={prDraft}
				className="!size-3.5 shrink-0"
			/>
			<span className="truncate max-w-[120px] hidden md:inline">
				{prTitle || (prNumberMatch ? `#${prNumberMatch}` : "PR")}
			</span>
			<span className="md:hidden">{prNumberMatch ? prNumberMatch : "PR"}</span>
		</a>
	);
};
