import {
	compactionThresholdLabel as bindingCompactionThresholdLabel,
	isCompactionPointBeyondWindow,
	type ResolvedCompactionThreshold,
} from "../compactionTriggers";
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

const compactionThresholdUnavailableLabel =
	"Auto-compaction threshold unavailable";

const isKnownContextLimit = (
	contextLimit: number | undefined,
): contextLimit is number =>
	contextLimit !== undefined &&
	Number.isFinite(contextLimit) &&
	contextLimit > 0;

/** Match chatloop normalization of out-of-range chat thresholds. */
const normalizeChatCompactionThreshold = (
	threshold: ResolvedCompactionThreshold,
): ResolvedCompactionThreshold =>
	threshold.source !== "organization" &&
	(threshold.percent < 0 || threshold.percent > 100)
		? { ...threshold, percent: 70 }
		: threshold;

/**
 * Describes whether and when automatic compaction is enabled, measuring an
 * organization override's point against the displayed context window.
 */
export const compactionThresholdLabel = (
	threshold: ResolvedCompactionThreshold | undefined,
	contextLimit: number | undefined,
): string => {
	if (threshold === undefined) return compactionThresholdUnavailableLabel;
	if (threshold.source === "organization") {
		if (!isKnownContextLimit(contextLimit))
			return compactionThresholdUnavailableLabel;
	} else if (!Number.isFinite(threshold.percent)) {
		return compactionThresholdUnavailableLabel;
	}
	return (
		bindingCompactionThresholdLabel(
			normalizeChatCompactionThreshold(threshold),
			contextLimit ?? 0,
		) ?? "Auto-compaction disabled"
	);
};

/** Token count at which the binding compaction trigger fires, when known. */
export const compactionThresholdTokens = (
	threshold: ResolvedCompactionThreshold | undefined,
	contextLimit: number | undefined,
): number | undefined => {
	if (threshold === undefined) return undefined;
	if (threshold.source !== "organization")
		return compactionTriggerTokens(contextLimit, threshold.percent);
	if (
		!isKnownContextLimit(contextLimit) ||
		isCompactionPointBeyondWindow(threshold.pointTokens, contextLimit)
	)
		return undefined;
	// The backend compacts once usage reaches the point.
	return Math.max(1, Math.ceil(threshold.pointTokens));
};
