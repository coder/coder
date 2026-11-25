import { formatBudgetUSD } from "#/utils/currency";
import { AIBudgetAmount } from "./AIBudgetAmount";

/** Spend against budget. Highlights spend once it nears or exceeds the limit; values in micros. */
export const AIBudgetUsage: React.FC<{
	currentSpend: number;
	spendLimit: number | null;
}> = ({ currentSpend, spendLimit }) => {
	if (spendLimit === null) {
		return (
			<span className="whitespace-nowrap">
				<span className="text-content-primary">
					{formatBudgetUSD(currentSpend)}
				</span>{" "}
				<span className="text-content-secondary">/ Unlimited USD</span>
			</span>
		);
	}

	return (
		<span className="whitespace-nowrap">
			<AIBudgetAmount spend={currentSpend} limit={spendLimit} />{" "}
			<span className="text-content-secondary">
				/ {formatBudgetUSD(spendLimit)} USD
			</span>
		</span>
	);
};
