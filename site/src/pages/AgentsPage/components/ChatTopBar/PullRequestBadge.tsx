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
	diffStatusData: {
		url,
		pull_request_state,
		pull_request_draft,
		pull_request_title,
		pr_number,
	},
	hiddenWhenPanelOpen,
}) => {
	const prNumber = pr_number?.toString() ?? parsePullRequestUrl(url)?.number;

	if (!url || !(pull_request_state || prNumber)) {
		return null;
	}

	const label = pull_request_title || (prNumber ? `#${prNumber}` : "PR");

	return (
		<a
			href={url}
			target="_blank"
			rel="noreferrer"
			className={cn(
				"inline-flex shrink-0 items-center gap-1.5 rounded-md border border-solid border-border-default px-2 py-0.5 text-xs font-medium text-content-secondary no-underline transition-colors hover:bg-surface-secondary hover:text-content-primary",
				hiddenWhenPanelOpen && "md:hidden",
			)}
		>
			<PrStateIcon
				state={pull_request_state}
				draft={pull_request_draft}
				className="!size-3.5 shrink-0"
			/>
			<span className="truncate max-w-[120px] hidden md:inline">{label}</span>
			<span className="md:hidden">{prNumber || "PR"}</span>
		</a>
	);
};
