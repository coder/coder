import { CalendarIcon, CircleDollarSignIcon, UsersIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import type { To } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { formatCustomLabel } from "#/components/DateTimeRangePicker/dateTimeRange";
import { formatCostMicros } from "#/utils/currency";
import { UnpricedModelsWarning } from "./UnpricedModelsWarning";

type SpendSummaryProps = {
	report: TypesGen.OrganizationAISpendReport;
	period: { start: Date; end: Date };
	unpricedModels: readonly string[] | undefined;
	setPricingHref: To | undefined;
};

/**
 * Summarizes every user matching the filters, not only the current page.
 */
export const SpendSummary: FC<SpendSummaryProps> = ({
	report,
	period,
	unpricedModels,
	setPricingHref,
}) => (
	<section
		aria-label="Spend summary"
		className="grid grid-cols-1 gap-4 sm:grid-cols-3 lg:max-w-3xl"
	>
		<SummaryCard
			value={formatCostMicros(report.totals.cost_micros)}
			label="Total spend"
			icon={<CircleDollarSignIcon />}
		>
			{report.totals.unpriced_usage_count > 0 && (
				<UnpricedModelsWarning
					label="Model pricing missing"
					models={unpricedModels}
					setPricingHref={setPricingHref}
				>
					Model pricing missing
				</UnpricedModelsWarning>
			)}
		</SummaryCard>
		<SummaryCard
			value={report.count.toLocaleString()}
			label={report.count === 1 ? "User" : "Users"}
			icon={<UsersIcon />}
		/>
		<SummaryCard
			value={formatCustomLabel(period.start, period.end)}
			label="Timeframe"
			icon={<CalendarIcon />}
		/>
	</section>
);

type SummaryCardProps = {
	value: string;
	label: string;
	icon: ReactNode;
	children?: ReactNode;
};

const SummaryCard: FC<SummaryCardProps> = ({
	value,
	label,
	icon,
	children,
}) => (
	<div className="flex min-w-0 flex-col gap-3 rounded-lg border border-solid border-border p-4">
		<div className="flex items-start justify-between gap-2">
			<div className="flex min-w-0 flex-col gap-1">
				<span className="truncate text-xl font-medium tabular-nums text-content-primary">
					{value}
				</span>
				<span className="text-sm text-content-secondary">{label}</span>
			</div>
			<span
				aria-hidden
				className="text-content-secondary [&_svg]:size-4 [&_svg]:shrink-0"
			>
				{icon}
			</span>
		</div>
		{children}
	</div>
);
