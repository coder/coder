import type { FC } from "react";
import { UsageBar } from "#/components/UsageBar/UsageBar";
import {
	formatSpendPeriodLabel,
	getSeverity,
	usageProgressPercentage,
} from "#/utils/budget";
import { formatBudgetUSD } from "#/utils/currency";

interface UserDropdownAISpendProps {
	currentSpend: number;
	/** A null limit means unlimited. */
	spendLimit: number | null;
	periodStart: string;
	periodEnd: string;
}

export const UserDropdownAISpend: FC<UserDropdownAISpendProps> = ({
	currentSpend,
	spendLimit,
	periodStart,
	periodEnd,
}) => {
	return (
		<div className="px-2 py-2">
			<div className="whitespace-nowrap text-sm text-content-primary">
				{formatBudgetUSD(currentSpend)}{" "}
				<span className="text-content-secondary">
					/ {spendLimit === null ? "Unlimited" : formatBudgetUSD(spendLimit)}{" "}
					USD
				</span>
			</div>
			{spendLimit !== null && (
				<UsageBar
					ariaLabel="AI spend usage"
					percent={usageProgressPercentage(currentSpend, spendLimit)}
					severity={getSeverity(currentSpend, spendLimit)}
					className="mt-2 h-2.5"
				/>
			)}
			<div className="mt-1 text-xs text-content-secondary">
				Estimated AI spend: {formatSpendPeriodLabel(periodStart, periodEnd)}
			</div>
		</div>
	);
};
