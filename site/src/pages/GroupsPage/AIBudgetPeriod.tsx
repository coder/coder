import dayjs from "dayjs";
import type { FC } from "react";
import { useQuery } from "react-query";
import { meAISpend } from "#/api/queries/users";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";

/** The current AI budget window, e.g. "June 1 - July 1, 2026". */
export const AIBudgetPeriod: FC = () => {
	const visible = Boolean(useFeatureVisibility().aibridge);
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
