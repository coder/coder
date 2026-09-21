import { cn } from "cn";
import { GitPullRequestArrowIcon } from "lucide-react";
import type { FC } from "react";
import type { ChatDiffStatus } from "#/api/typesGenerated";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { parsePullRequestUrl } from "../../../utils/pullRequest";
import { getPRIconConfig } from "./statusConfig";

type ChatNodePRIconProps = {
	readonly prStatuses: ChatDiffStatus[];
};

export const ChatNodePRIcon: FC<ChatNodePRIconProps> = ({ prStatuses }) => {
	// One PR shows that PR's state. Several PRs share the count
	// glyph: picking one state would misrepresent the rest.
	if (prStatuses.length === 0) {
		return null;
	}
	if (prStatuses.length === 1) {
		const soleConfig = getPRIconConfig(prStatuses[0]);
		if (!soleConfig) {
			return null;
		}
		const SoleIcon = soleConfig.icon;
		return (
			<SoleIcon
				role="img"
				aria-label={soleConfig.label}
				className={cn("size-3.5 shrink-0", soleConfig.className)}
			/>
		);
	}

	return (
		<Tooltip>
			<TooltipTrigger
				asChild
				className="inline-flex shrink-0 items-center gap-0.5"
			>
				<span>
					<span className="text-[13px] leading-4 tabular-nums">
						{prStatuses.length}
					</span>
					<GitPullRequestArrowIcon
						role="img"
						aria-label={`${prStatuses.length} pull requests`}
						className="size-3.5 shrink-0 text-content-secondary"
					/>
				</span>
			</TooltipTrigger>
			<TooltipContent
				side="bottom"
				className="flex max-w-72 flex-col gap-2 p-3"
			>
				{prStatuses.map((status, index) => {
					const config = getPRIconConfig(status);
					if (!config) {
						return null;
					}
					const Icon = config.icon;

					// The label, trimmed, with a URL fallback so the row
					// always shows something readable.
					const label =
						status.pull_request_title.trim() || status.url || "Pull request";

					// Prefer the stored number; parse it from the URL when
					// the row predates the column.
					const parsed = parsePullRequestUrl(status.url);
					const prNumber =
						status.pr_number ?? (parsed && Number(parsed.number));

					return (
						<div
							key={`${status.remote_origin ?? ""}/${status.git_branch ?? ""}/${index}`}
							className="flex items-center gap-1.5 text-left"
						>
							<Icon className={cn("size-3.5 shrink-0", config.className)} />
							<span className="shrink-0 font-semibold">
								PR {prNumber ? `#${prNumber}` : label}
							</span>
							<span className="min-w-0 truncate">{label}</span>
						</div>
					);
				})}
			</TooltipContent>
		</Tooltip>
	);
};
