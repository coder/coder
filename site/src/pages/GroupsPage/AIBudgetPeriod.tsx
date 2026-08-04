import type { FC } from "react";
import { useQuery } from "react-query";
import { meAISpend } from "#/api/queries/users";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { formatSpendPeriodLabel } from "#/utils/budget";

/** The current AI budget window, e.g. "June 1 - July 1, 2026". */
export const AIBudgetPeriod: FC = () => {
	const visible = Boolean(useFeatureVisibility().aibridge);
	const { data: aiSpend } = useQuery({ ...meAISpend(), enabled: visible });

	if (!visible || !aiSpend) {
		return null;
	}

	return (
		<span className="text-sm text-content-secondary">
			AI budget period:{" "}
			{formatSpendPeriodLabel(aiSpend.period_start, aiSpend.period_end)}
		</span>
	);
};
