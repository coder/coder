import type { FC } from "react";
import { getSeverity, severityTextClassName } from "#/utils/budget";
import { formatBudgetUSD } from "#/utils/currency";

/** Spend against budget. Highlights spend once it nears or exceeds the limit; values in micros. */
export const AIBudgetUsage: FC<{
	currentSpend: number;
	spendLimit: number | null;
}> = ({ currentSpend, spendLimit }) => {
	if (spendLimit === null) {
		return (
			<span className="whitespace-nowrap">
				{formatBudgetUSD(currentSpend)}{" "}
				<span className="text-content-disabled">/ Unlimited USD</span>
			</span>
		);
	}

	const severity = getSeverity(currentSpend, spendLimit);
	// Emphasize spend like the budget until it nears the limit.
	const spendClassName =
		severity === "normal"
			? "text-content-primary"
			: severityTextClassName(severity);
	return (
		<span className="whitespace-nowrap">
			<span className={spendClassName}>{formatBudgetUSD(currentSpend)}</span>{" "}
			<span className="text-content-primary">
				/ {formatBudgetUSD(spendLimit)}
			</span>{" "}
			<span className="text-content-disabled">USD</span>
		</span>
	);
};
