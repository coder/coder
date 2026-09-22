import type { AgentContextUsage } from "../components/ContextUsageIndicator";
import { compactionTriggerTokens } from "./modelOptions";

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

/** Textual equivalent of the automatic-compaction budget, including unknown states. */
export const contextUsageLabel = (usage: AgentContextUsage | null): string => {
	const used = usage?.usedTokens;
	const hasUsage = used !== undefined && Number.isFinite(used) && used >= 0;
	const threshold = usage?.compressionThreshold;
	const trigger = compactionTriggerTokens(usage?.contextLimitTokens, threshold);
	let label: string;
	if (threshold === 100) {
		label = hasUsage
			? `${formatContextTokenCount(used, true)} tokens used; automatic compaction is disabled`
			: "Context usage unavailable; automatic compaction is disabled";
	} else if (!hasUsage) {
		label = "Context usage unavailable";
	} else if (trigger === undefined) {
		label = `${formatContextTokenCount(used, true)} tokens used; auto-compaction budget unavailable`;
	} else {
		label = `${formatContextTokenCount(used, true)} of ${formatContextTokenCount(trigger, true)} before auto-compaction`;
	}
	return usage?.estimated
		? `${label} (estimated from compaction summary only)`
		: label;
};
