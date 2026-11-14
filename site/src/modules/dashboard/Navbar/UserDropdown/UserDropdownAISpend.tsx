import { OctagonAlertIcon } from "lucide-react";
import { UsageBar } from "#/components/UsageBar/UsageBar";
import {
	formatSpendPeriodEnd,
	formatSpendPeriodLabel,
	type UsageSeverity,
	usageProgressPercentage,
} from "#/utils/budget";
import { formatBudgetUSD } from "#/utils/currency";
import { getSpendSeverity, type ViewedSpend } from "./viewedSpend";

type UserDropdownAISpendProps = ViewedSpend & {
	periodStart: string;
	periodEnd: string;
};

export const UserDropdownAISpend: React.FC<UserDropdownAISpendProps> = ({
	currentSpend,
	spendLimit,
	periodStart,
	periodEnd,
}) => {
	const severity = getSpendSeverity({ currentSpend, spendLimit });
	const exceeded = severity === "exceeded";

	return (
		<div className="px-2 py-2">
			{exceeded && (
				<div className="mb-1 flex items-center gap-1.5 whitespace-nowrap text-sm">
					<OctagonAlertIcon
						aria-hidden
						className="size-icon-xs shrink-0 text-highlight-red"
					/>
					<span className="text-content-primary">AI spend</span>
					<span className="ml-auto text-xs text-highlight-red">
						Limit exceeded
					</span>
				</div>
			)}
			<SpendUsage
				currentSpend={currentSpend}
				spendLimit={spendLimit}
				severity={severity}
			/>
			{exceeded && (
				<div className="mt-1 text-xs text-content-primary">
					AI requests are paused until{" "}
					<span className="whitespace-nowrap">
						{formatSpendPeriodEnd(periodEnd)}.
					</span>
				</div>
			)}
			<div className="mt-1 text-xs text-content-secondary">
				Approximate AI spend{" "}
				<span className="whitespace-nowrap">
					{formatSpendPeriodLabel(periodStart, periodEnd)}
				</span>
			</div>
		</div>
	);
};

type SpendUsageProps = ViewedSpend & { severity: UsageSeverity };

const SpendUsage: React.FC<SpendUsageProps> = ({
	currentSpend,
	spendLimit,
	severity,
}) => {
	return (
		<>
			<div className="whitespace-nowrap text-sm text-content-primary">
				{formatBudgetUSD(currentSpend)}{" "}
				<span className="text-content-secondary">
					/ {spendLimit === null ? "Unlimited" : formatBudgetUSD(spendLimit)}{" "}
					USD
				</span>
			</div>
			{spendLimit !== null && (
				// A menu cannot own a progressbar, and the text above already states
				// the spend and limit, so the bar is hidden from assistive tech.
				<div aria-hidden>
					<UsageBar
						ariaLabel="AI spend usage"
						percent={usageProgressPercentage(currentSpend, spendLimit)}
						severity={severity}
						className="mt-2 h-2.5"
					/>
				</div>
			)}
		</>
	);
};
