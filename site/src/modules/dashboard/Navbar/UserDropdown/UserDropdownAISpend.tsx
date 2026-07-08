import type { FC, ReactNode } from "react";
import { UsageBar } from "#/components/UsageBar/UsageBar";
import { formatBudgetUSD } from "#/utils/currency";
import type { AISpend } from "./useAISpend";

interface UserDropdownAISpendProps {
	spend: AISpend | null;
	/** Rendered above the section, only when the section is shown. */
	header?: ReactNode;
}

export const UserDropdownAISpend: FC<UserDropdownAISpendProps> = ({
	spend,
	header,
}) => {
	if (!spend) {
		return null;
	}

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
