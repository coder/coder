import { useQuery } from "react-query";
import { meAISpend } from "#/api/queries/users";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import {
	getSeverity,
	type UsageSeverity,
	usageProgressPercentage,
} from "#/utils/budget";

export interface AISpend {
	currentSpend: number;
	/** A null limit means unlimited. */
	spendLimit: number | null;
	percent: number;
	severity: UsageSeverity;
}

/** Resolves AI spend for the avatar border and dropdown section, or null when
 * it should be hidden. */
export function useAISpend(): AISpend | null {
	const { experiments } = useDashboard();
	// TODO(AIGOV-443): drop the experiment gate once cost control is stable.
	const aibridgeVisible =
		useFeatureVisibility().aibridge &&
		experiments.includes("ai-gateway-cost-control");
	const { data, isError } = useQuery({
		...meAISpend(),
		enabled: aibridgeVisible,
	});

	if (!aibridgeVisible || isError || !data) {
		return null;
	}

	const currentSpend = data.current_spend_micros;
	const spendLimit = data.spend_limit_micros;

	// Hide on invalid spend data. A null limit means unlimited, which is shown.
	if (currentSpend < 0 || (spendLimit !== null && spendLimit < 0)) {
		return null;
	}

	return {
		currentSpend,
		spendLimit,
		percent:
			spendLimit === null
				? 0
				: usageProgressPercentage(currentSpend, spendLimit),
		severity:
			spendLimit === null ? "normal" : getSeverity(currentSpend, spendLimit),
	};
}
