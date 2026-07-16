import type { FC } from "react";
import { AIBudgetAmount } from "#/components/AIBudgetAmount/AIBudgetAmount";
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

	return (
		<span className="whitespace-nowrap">
			<AIBudgetAmount spend={currentSpend} limit={spendLimit} />{" "}
			<span className="text-content-primary">
				/ {formatBudgetUSD(spendLimit)}
			</span>{" "}
			<span className="text-content-disabled">USD</span>
		</span>
	);
};
