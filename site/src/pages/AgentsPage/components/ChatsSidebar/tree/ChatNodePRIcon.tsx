import { cn } from "cn";
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
	readonly chatID: string;
	readonly prStatuses: ChatDiffStatus[];
};

export const ChatNodePRIcon: FC<ChatNodePRIconProps> = ({
	chatID,
	prStatuses,
}) => {
	const primary = prStatuses[0];
	const primaryConfig = getPRIconConfig(primary);
	if (!primaryConfig) {
		return null;
	}
	const PrimaryIcon = primaryConfig.icon;

	if (prStatuses.length === 1) {
		return (
			<PrimaryIcon
				role="img"
				aria-label={primaryConfig.label}
				className={cn("size-3.5 shrink-0", primaryConfig.className)}
			/>
		);
	}

	const prLabel = (status: ChatDiffStatus): string =>
		status.pull_request_title.trim() || status.url || "Pull request";

	return (
		<Tooltip>
			<TooltipTrigger
				asChild
				className="inline-flex shrink-0 items-center gap-0.5"
				data-testid={`chat-node-pr-trigger-${chatID}`}
			>
				<span>
					<span className="text-[13px] leading-4 tabular-nums">
						{prStatuses.length}
					</span>
					<PrimaryIcon
						role="img"
						aria-label={`${prStatuses.length} pull requests`}
						className={cn("size-3.5 shrink-0", primaryConfig.className)}
					/>
				</span>
			</TooltipTrigger>
			<TooltipContent
				side="bottom"
				className="flex max-w-72 flex-col gap-2 p-3"
				data-testid={`chat-node-pr-list-${chatID}`}
			>
				{prStatuses.map((status, index) => {
					const config = getPRIconConfig(status);
					if (!config) {
						return null;
					}
					const Icon = config.icon;
					const parsed = parsePullRequestUrl(status.url);
					const prNumber =
						status.pr_number ?? (parsed ? Number(parsed.number) : undefined);
					const prName = prNumber ? `#${prNumber}` : prLabel(status);
					return (
						<div
							key={`${status.remote_origin ?? ""}/${status.git_branch ?? ""}/${index}`}
							className="flex items-center gap-1.5 text-left"
						>
							<Icon className={cn("size-3.5 shrink-0", config.className)} />
							<span className="shrink-0 font-semibold">PR {prName}</span>
							<span className="min-w-0 truncate">{prLabel(status)}</span>
						</div>
					);
				})}
			</TooltipContent>
		</Tooltip>
	);
};
