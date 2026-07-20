import dayjs from "dayjs";
import type { FC } from "react";
import { useQuery } from "react-query";
import { meAISpend } from "#/api/queries/users";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";

/** The current AI budget window, e.g. "June 1 - July 1, 2026". */
export const AIBudgetPeriod: FC = () => {
	const { experiments } = useDashboard();
	// TODO(AIGOV-443): drop the experiment gate once cost control is stable.
	const visible =
		Boolean(useFeatureVisibility().aibridge) &&
		experiments.includes("ai-gateway-cost-control");
	const { data: aiSpend } = useQuery({ ...meAISpend(), enabled: visible });

	if (!visible || !aiSpend) {
		return null;
	}

	// Local time and raw exclusive period_end, matching the spend page.
	const start = dayjs(aiSpend.period_start).format("MMMM D");
	const end = dayjs(aiSpend.period_end).format("MMMM D, YYYY");
	return (
		<span className="text-sm text-content-secondary">
			AI budget period: {start} - {end}
		</span>
	);
};
