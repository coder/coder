import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import { meAISpend } from "#/api/queries/users";
import { UsageBar } from "#/components/UsageBar/UsageBar";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { getSeverity, usageProgressPercentage } from "#/utils/budget";
import { formatBudgetUSD } from "#/utils/currency";

interface UserDropdownAISpendProps {
	/** Rendered above the section, only when the section is shown. */
	header?: ReactNode;
}

export const UserDropdownAISpend: FC<UserDropdownAISpendProps> = ({
	header,
}) => {
	const { experiments } = useDashboard();
	// TODO(AIGOV-443): drop the experiment gate once cost control is stable.
	const aibridgeVisible =
		useFeatureVisibility().aibridge &&
		experiments.includes("ai-gateway-cost-control");
	const { data, isError } = useQuery({
		...meAISpend(),
		enabled: aibridgeVisible,
	});

	if (!aibridgeVisible || isError || !data) {
		return null;
	}

	const spendLimit = data.spend_limit_micros;
	const currentSpend = data.current_spend_micros;

	// Hide on invalid spend data. A null limit means unlimited, which is shown.
	if (currentSpend < 0 || (spendLimit !== null && spendLimit < 0)) {
		return null;
	}

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
						percent={usageProgressPercentage(currentSpend, spendLimit)}
						severity={getSeverity(currentSpend, spendLimit)}
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
