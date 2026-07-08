import dayjs from "dayjs";
import type { FC } from "react";
import { useQuery } from "react-query";
import { meAISpend } from "#/api/queries/users";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";

/** The current AI budget window, e.g. "June 1 - June 30, 2026". */
export const AIBudgetPeriod: FC = () => {
	const { experiments } = useDashboard();
	// TODO(AIGOV-443): drop the experiment gate once cost control is stable.
	const visible =
		useFeatureVisibility().aibridge &&
		experiments.includes("ai-gateway-cost-control");
	const { data: aiSpend } = useQuery({ ...meAISpend(), enabled: visible });

	if (!visible || !aiSpend) {
		return null;
	}

	const start = dayjs(aiSpend.period_start).format("MMMM D");
	// period_end is exclusive, so the inclusive window ends the day before.
	const end = dayjs(aiSpend.period_end)
		.subtract(1, "day")
		.format("MMMM D, YYYY");
	return (
		<span className="text-sm text-content-secondary">
			{`AI budget period: ${start} - ${end}`}
		</span>
	);
};
