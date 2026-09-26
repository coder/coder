import { cn } from "cn";
import { GitPullRequestArrowIcon } from "lucide-react";
import type { FC } from "react";
import type { ChatDiffStatus } from "#/api/typesGenerated";
import { getPRIconConfig } from "./statusConfig";

type ChatNodePRIconProps = {
	readonly prStatuses: ChatDiffStatus[];
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
			<span aria-hidden="true" className="text-[13px] leading-4 tabular-nums">
				{prStatuses.length}
			</span>
			<GitPullRequestArrowIcon
				aria-hidden="true"
				className="size-3.5 shrink-0 text-content-secondary"
			/>
		</span>
	);
};
