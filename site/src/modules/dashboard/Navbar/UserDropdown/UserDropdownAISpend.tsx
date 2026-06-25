import type { FC } from "react";
import { useQuery } from "react-query";
import { meAISpend } from "#/api/queries/users";
import { DropdownMenuSeparator } from "#/components/DropdownMenu/DropdownMenu";
import { UsageBar } from "#/components/UsageBar/UsageBar";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { getSeverity, usageProgressPercentage } from "#/utils/budget";
import { formatBudgetUSD } from "#/utils/currency";

export const UserDropdownAISpend: FC = () => {
	const { experiments } = useDashboard();
	// TODO(AIGOV-443): drop the experiment gate once cost control is stable.
	const aibridgeVisible =
		useFeatureVisibility().aibridge &&
		experiments.includes("ai-gateway-cost-control");
	const { data, isError } = useQuery({
		...meAISpend(),
		enabled: aibridgeVisible,
	});

	const spendLimit = data?.spend_limit_micros;
	const currentSpend = data?.current_spend_micros;

	// Hide unless the add-on is on and the user has a non-negative budget set.
	if (
		!aibridgeVisible ||
		isError ||
		spendLimit == null ||
		currentSpend === undefined ||
		spendLimit < 0 ||
		currentSpend < 0
	) {
		return null;
	}

	const percent = usageProgressPercentage(currentSpend, spendLimit);
	const severity = getSeverity(currentSpend, spendLimit);

	return (
		<>
			<DropdownMenuSeparator />
			<div className="px-2 py-2">
				<div className="mb-2 text-sm font-medium text-content-secondary">
					AI spend - {formatBudgetUSD(currentSpend)} /{" "}
					{formatBudgetUSD(spendLimit)} USD
				</div>
				<UsageBar
					ariaLabel="AI spend usage"
					percent={percent}
					severity={severity}
					className="h-2.5"
				/>
				<span className="sr-only">{Math.round(percent)}% used</span>
			</div>
		</>
	);
};
