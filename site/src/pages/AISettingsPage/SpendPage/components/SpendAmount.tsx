import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
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
				<TooltipContent side="bottom" className="max-w-xs">
					Some users have used models without configured pricing. That usage is
					excluded, so total spend may be higher than shown.
				</TooltipContent>
			</Tooltip>
		);
	}
	return (
		<InfoTooltip
			type="warning"
			size="small"
			ariaLabel={`Cost setup for ${scope.user}`}
		>
			This user has used models without configured pricing. That usage is
			excluded, so their actual spend may be higher than shown.
		</InfoTooltip>
	);
};

type SpendAmountProps = {
	costMicros: number;
	unpricedUsageCount: number;
	scope: SpendScope;
};

/** Shows spend as a lower bound when model pricing is missing. */
export const SpendAmount: FC<SpendAmountProps> = ({
	costMicros,
	unpricedUsageCount,
	scope,
}) => {
	return (
		<span className="inline-flex items-center gap-2 tabular-nums">
			{formatCostMicros(costMicros)}
			{unpricedUsageCount > 0 && <CostSetupWarning scope={scope} />}
		</span>
	);
};
