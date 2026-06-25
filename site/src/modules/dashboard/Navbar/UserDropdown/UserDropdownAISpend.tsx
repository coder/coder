import type { FC } from "react";
import { useQuery } from "react-query";
import { meAISpend } from "#/api/queries/users";
import {
	clampPercentage,
	getSeverity,
	severityProgressClassName,
	type UsageSeverity,
	usageProgressPercentage,
} from "#/utils/budget";
import { cn } from "#/utils/cn";
import { formatBudgetUSD } from "#/utils/currency";

export const UserDropdownAISpend: FC = () => {
	const { data, isError } = useQuery(meAISpend());

	if (isError || data?.spend_limit_micros === undefined) {
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
	const roundedPercent = Math.round(percent);
	const severity = getSeverity(currentSpend, spendLimit);

	return (
		<div className="px-2 pb-2 pt-0.5">
			<div className="mb-2 text-xs font-medium text-content-secondary">
				AI spend - {formatBudgetUSD(currentSpend)} /{" "}
				{formatBudgetUSD(spendLimit)} USD
			</div>
			<SpendProgress percent={percent} severity={severity} />
			<span className="sr-only">{roundedPercent}% used</span>
		</div>
	);
};

const SpendProgress: FC<{
	percent: number;
	severity: UsageSeverity;
}> = ({ percent, severity }) => {
	const clampedPercent = clampPercentage(percent);

	return (
		<div
			role="progressbar"
			aria-label="AI spend usage"
			aria-valuemin={0}
			aria-valuemax={100}
			aria-valuenow={Math.round(clampedPercent)}
			className="h-1.5 overflow-hidden rounded-full bg-surface-tertiary"
		>
			<div
				className={cn(
					"h-full rounded-full transition-all duration-300 ease-out",
					severityProgressClassName(severity),
				)}
				style={{ width: `${clampedPercent}%` }}
			/>
		</div>
	);
};
