import dayjs from "dayjs";
import type { FC } from "react";
import { useAIBudgetVisible } from "./useAIBudgetVisible";

/** The current AI budget window, e.g. "June 1 - June 30, 2026". */
export const AIBudgetPeriod: FC = () => {
	const { visible, aiSpend } = useAIBudgetVisible();

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
