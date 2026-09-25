import { cn } from "cn";
import { GitPullRequestArrowIcon } from "lucide-react";
import type { FC } from "react";
import type { ChatDiffStatus } from "#/api/typesGenerated";
import { TooltipContent } from "#/components/Tooltip/Tooltip";
import { parsePullRequestUrl } from "../../../utils/pullRequest";
import { getPRIconConfig } from "./statusConfig";

type ChatNodePRIconProps = {
	readonly prStatuses: ChatDiffStatus[];
};

/** The PR number, parsed from the URL for legacy rows that predate pr_number. */
export const getPullRequestNumber = (
	status: ChatDiffStatus,
): number | undefined => {
	if (status.pr_number) {
		return status.pr_number;
	}
	const parsed = Number(parsePullRequestUrl(status.url)?.number);
	return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
};

export const ChatNodePRIcon: FC<ChatNodePRIconProps> = ({ prStatuses }) => {
	// Several PRs share the count glyph: picking one state would
	// misrepresent the rest.
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

	// One name for the whole glyph; the count and the icon carry it
	// visually only.
	return (
		<span
			role="img"
			aria-label={`${prStatuses.length} pull requests`}
			className="inline-flex shrink-0 items-center gap-0.5"
		>
			<span
				aria-hidden="true"
				className="text-(length:--agent-font-size) leading-4 tabular-nums"
			>
				{prStatuses.length}
			</span>
			<GitPullRequestArrowIcon
				aria-hidden="true"
				className="size-3.5 shrink-0 text-content-secondary"
			/>
		</span>
	);
};

type PRListContentProps = {
	readonly prStatuses: ChatDiffStatus[];
};

// The tooltip body listing every tracked PR. ChatTreeNode renders it
// through the link trigger, so it must not nest another trigger here.
export const PRListTooltipContent: FC<PRListContentProps> = ({
	prStatuses,
}) => {
	return (
		<TooltipContent side="bottom" className="flex max-w-72 flex-col gap-2 p-3">
			{prStatuses.map((status, index) => {
				const config = getPRIconConfig(status);
				if (!config) {
					return null;
				}
				const Icon = config.icon;

				const label =
					status.pull_request_title.trim() || status.url || "Pull request";

				const prNumber = getPullRequestNumber(status);

				return (
					<div
						key={`${status.remote_origin ?? ""}/${status.git_branch ?? ""}/${index}`}
						className="flex items-center gap-1.5 text-left"
					>
						<Icon className={cn("size-3.5 shrink-0", config.className)} />
						{/* The state reaches screen readers through the
							link's description; icon labels are skipped there,
							so the state rides as hidden text. */}
						<span className="sr-only">{config.label}</span>
						<span className="shrink-0 font-semibold">
							PR {prNumber ? `#${prNumber}` : label}
						</span>
						<span className="min-w-0 truncate">{label}</span>
					</div>
				);
			})}
		</TooltipContent>
	);
};
