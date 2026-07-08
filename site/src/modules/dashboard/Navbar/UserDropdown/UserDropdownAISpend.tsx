import type { FC, ReactNode } from "react";
import { UsageBar } from "#/components/UsageBar/UsageBar";
import type { UsageSeverity } from "#/utils/budget";
import { formatBudgetUSD } from "#/utils/currency";

export interface AISpend {
	currentSpend: number;
	/** A null limit means unlimited. */
	spendLimit: number | null;
	percent: number;
	severity: UsageSeverity;
}

interface UserDropdownAISpendProps {
	spend: AISpend;
	/** Rendered above the section, only when the section is shown. */
	header?: ReactNode;
}

export const UserDropdownAISpend: FC<UserDropdownAISpendProps> = ({
	spend,
	header,
}) => {
	const { currentSpend, spendLimit, percent, severity } = spend;

	return (
		<>
			{header}
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
						percent={percent}
						severity={severity}
						className="mt-2 h-2.5"
					/>
				)}
				<div className="mt-1 text-xs text-content-secondary">
					(AI spend/month)
				</div>
			</div>
		</>
	);
};
