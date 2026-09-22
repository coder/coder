import type { FC } from "react";
import { formatContextTokenCount } from "../utils/contextUsage";
import { compactionTriggerTokens } from "../utils/modelOptions";
import type { AgentContextUsage } from "./ContextUsageIndicator";

export const ChatDetailsUsage: FC<{
	usage: AgentContextUsage | null;
	compact?: boolean;
}> = ({ usage, compact = false }) => {
	const used = usage?.usedTokens;
	const limit = usage?.contextLimitTokens;
	const percent =
		used !== undefined &&
		Number.isFinite(used) &&
		used >= 0 &&
		limit !== undefined &&
		Number.isFinite(limit) &&
		limit > 0
			? (used / limit) * 100
			: undefined;
	const percentLabel =
		percent === undefined ? "Unknown" : `${Math.round(percent)}%`;
	const threshold = usage?.compressionThreshold;
	const thresholdLabel =
		threshold === undefined || !Number.isFinite(threshold)
			? "Auto-compaction threshold unavailable"
			: threshold === 100
				? "Auto-compaction disabled"
				: "Compacts at " +
					(threshold < 0 || threshold > 100 ? 70 : threshold) +
					"%";
	const trigger =
		threshold !== undefined && limit !== undefined
			? compactionTriggerTokens(limit, threshold)
			: undefined;
	const tokensLabel =
		formatContextTokenCount(used) +
		" of " +
		formatContextTokenCount(limit) +
		" tokens used";
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
		return (
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
				<span>{percentLabel}</span>
			</div>
			{bar}
			<p className="m-0 text-xs text-content-secondary">
				{percent === undefined ? "Context usage unavailable. " : ""}
				{tokensLabel}.
			</p>
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
