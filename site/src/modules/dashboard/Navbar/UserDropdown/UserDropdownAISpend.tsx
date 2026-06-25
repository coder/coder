import type { FC } from "react";
import { useQuery } from "react-query";
import { meAISpend } from "#/api/queries/users";
import { UsageBar } from "#/components/UsageBar/UsageBar";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { getSeverity, usageProgressPercentage } from "#/utils/budget";
import { formatBudgetUSD } from "#/utils/currency";

export const UserDropdownAISpend: FC = () => {
	const { experiments } = useDashboard();
	// TODO(AIGOV-443): remove the ai-gateway-cost-control experiment gate once
	// the cost-control feature is stable.
	const aibridgeVisible =
		Boolean(useFeatureVisibility().aibridge) &&
		experiments.includes("ai-gateway-cost-control");
	const { data, isError } = useQuery({
		...meAISpend(),
		enabled: aibridgeVisible,
	});

	if (!aibridgeVisible || isError || data?.spend_limit_micros === undefined) {
		return null;
	}

	const currentSpend = data.current_spend_micros;
	const spendLimit = data.spend_limit_micros;

	if (
		spendLimit === null ||
		!Number.isFinite(currentSpend) ||
		!Number.isFinite(spendLimit) ||
		currentSpend < 0 ||
		spendLimit < 0
	) {
		return null;
	}

	const percent = usageProgressPercentage(currentSpend, spendLimit);
	const severity = getSeverity(currentSpend, spendLimit);

	return (
		<div className="px-2 pb-2 pt-0.5">
			<div className="mb-2 text-xs font-medium text-content-secondary">
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
	);
};
