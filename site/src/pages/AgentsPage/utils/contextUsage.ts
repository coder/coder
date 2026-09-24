import type { AgentContextUsage } from "../components/ContextUsageIndicator";

/** Formats context counts consistently in the ring and Details panel. */
export const formatContextTokenCount = (
	value: number | undefined,
	compact = false,
): string => {
	if (value === undefined || !Number.isFinite(value) || value < 0)
		return "Unknown";
	if (!compact || value < 1_000) return value.toLocaleString("en-US");
	const divisor = value >= 1_000_000 ? 1_000_000 : 1_000;
	return `${Number((value / divisor).toFixed(1))}${divisor === 1_000_000 ? "M" : "K"}`;
};

/** Describes usage against the full context window, including unavailable data. */
export const contextUsageLabel = (usage: AgentContextUsage | null): string => {
	const used = usage?.usedTokens;
	const hasUsage = used !== undefined && Number.isFinite(used) && used >= 0;
	const limit = usage?.contextLimitTokens;
	const hasLimit = limit !== undefined && Number.isFinite(limit) && limit > 0;
	let label: string;
	if (!hasUsage) {
		label = "Context usage unavailable";
	} else if (!hasLimit) {
		label = `${formatContextTokenCount(used, true)} tokens used; context window size unavailable`;
	} else {
		const percent = Math.round((used / limit) * 100);
		label = `${percent}% - ${formatContextTokenCount(used, true)} / ${formatContextTokenCount(limit, true)} context used`;
	}
	return usage?.estimated
		? `${label} (estimated from compaction summary only)`
		: label;
};

/** Describes whether and when automatic compaction is enabled. */
export const compactionThresholdLabel = (
	threshold: number | undefined,
): string => {
	if (threshold === undefined || !Number.isFinite(threshold))
		return "Auto-compaction threshold unavailable";
	if (threshold === 100) return "Auto-compaction disabled";
	return `Compacts at ${threshold < 0 || threshold > 100 ? 70 : threshold}%`;
};
