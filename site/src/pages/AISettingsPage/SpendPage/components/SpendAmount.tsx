import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatCostMicros } from "#/utils/currency";

/**
 * Whose unpriced usage the warning describes. Each user row names its user
 * so the warnings stay distinguishable by accessible name.
 */
type SpendScope = "organization" | { user: string };

/**
 * The total gets a labeled badge; user rows get only the icon so the compact
 * cells stay readable.
 */
const CostSetupWarning: FC<{ scope: SpendScope }> = ({ scope }) => {
	if (scope === "organization") {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<Badge asChild variant="warning" size="sm" hover>
						<button
							type="button"
							aria-label="Cost setup for total spend"
							className="cursor-default font-medium"
						>
							<TriangleAlertIcon />
							Cost setup
						</button>
					</Badge>
				</TooltipTrigger>
				<TooltipContent side="bottom" align="start" className="max-w-xs">
					Some users have used models without configured pricing. That usage is
					excluded, so total spend may be higher than shown.
				</TooltipContent>
			</Tooltip>
		);
	}
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					aria-label={`Cost setup for ${scope.user}`}
					className="flex cursor-default items-center border-0 bg-transparent p-0 text-content-warning opacity-75 transition-opacity hover:opacity-100 [&_svg]:size-3"
				>
					<TriangleAlertIcon />
				</button>
			</TooltipTrigger>
			{/* Spend cells are right-aligned, so the tooltip hangs off the table edge. */}
			<TooltipContent side="bottom" align="end" className="max-w-xs">
				This user has used models without configured pricing. That usage is
				excluded, so their actual spend may be higher than shown.
			</TooltipContent>
		</Tooltip>
	);
};

type SpendAmountProps = {
	costMicros: number;
	unpricedUsageCount: number;
	scope: SpendScope;
};

/**
 * Shows spend as a lower bound when model pricing is missing. The row icon
 * sits before the amount so right-aligned figures stay lined up; the total's
 * badge follows it.
 */
export const SpendAmount: FC<SpendAmountProps> = ({
	costMicros,
	unpricedUsageCount,
	scope,
}) => {
	const warning = unpricedUsageCount > 0 && <CostSetupWarning scope={scope} />;
	return (
		<span className="inline-flex items-center gap-2 tabular-nums">
			{scope !== "organization" && warning}
			{formatCostMicros(costMicros)}
			{scope === "organization" && warning}
		</span>
	);
};
