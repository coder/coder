import type { FC } from "react";
import {
	compactionThresholdLabel,
	formatContextTokenCount,
} from "../utils/contextUsage";
import { compactionTriggerTokens } from "../utils/modelOptions";
import type { AgentContextUsage } from "./ContextUsageIndicator";

export const ChatDetailsUsage: FC<{
	usage: AgentContextUsage | null;
	compact?: boolean;
}> = ({ usage, compact = false }) => {
	const used = usage?.usedTokens;
	const limit = usage?.contextLimitTokens;
	const hasUsage = used !== undefined && Number.isFinite(used) && used >= 0;
	const hasLimit = limit !== undefined && Number.isFinite(limit) && limit > 0;
	const percent = hasUsage && hasLimit ? (used / limit) * 100 : undefined;
	const percentLabel =
		percent === undefined ? undefined : `${Math.round(percent)}%`;
	const threshold = usage?.compressionThreshold;
	const thresholdLabel = compactionThresholdLabel(threshold);
	const trigger =
		threshold !== undefined && limit !== undefined
			? compactionTriggerTokens(limit, threshold)
			: undefined;
	const tokensLabel = hasUsage
		? hasLimit
			? `${formatContextTokenCount(used)} of ${formatContextTokenCount(limit)} tokens used.`
			: `${formatContextTokenCount(used)} tokens used. Context window size unavailable.`
		: hasLimit
			? `Token usage unavailable. Context window: ${formatContextTokenCount(limit)} tokens.`
			: "Token usage and context window size unavailable.";
	const bar = (
		<span
			aria-hidden="true"
			className="block h-1.5 min-w-8 overflow-hidden rounded-full bg-surface-quaternary forced-colors:border forced-colors:border-solid"
		>
			<span
				className="block h-full rounded-full bg-content-link forced-colors:bg-[Highlight]"
				style={{
					width:
						(percent === undefined ? 0 : Math.min(100, Math.max(0, percent))) +
						"%",
				}}
			/>
		</span>
	);
	if (compact)
		return percent === undefined ? null : (
			<span className="flex w-24 max-w-full items-center gap-2 text-xs font-normal text-content-secondary">
				<span className="min-w-8 flex-1">{bar}</span>
				<span>{percentLabel}</span>{" "}
				<span className="sr-only">
					context used{usage?.estimated ? ", estimated" : ""}
				</span>
			</span>
		);
	return (
		<div className="flex flex-col gap-2">
			<div className="flex flex-wrap items-center justify-between gap-2">
				<span>
					{usage?.estimated ? "Estimated context usage" : "Context usage"}
				</span>
				{percentLabel && <span>{percentLabel}</span>}
			</div>
			{percent !== undefined && bar}
			<p className="m-0 text-xs text-content-secondary">{tokensLabel}</p>
			<p className="m-0 text-xs text-content-secondary">
				{thresholdLabel}
				{trigger !== undefined && threshold !== 100
					? ` (${formatContextTokenCount(trigger)} tokens)`
					: ""}
				.
			</p>
			{usage?.estimated && (
				<p className="m-0 text-xs text-content-secondary">
					Based on the compacted summary only, excluding other prompt content
					and tools. Replaced by measured usage after the next response.
				</p>
			)}
		</div>
	);
};
