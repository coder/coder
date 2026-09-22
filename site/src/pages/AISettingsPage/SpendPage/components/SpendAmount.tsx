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
 * Whose unpriced usage the badge describes. Each user row names its user so
 * the badges stay distinguishable by accessible name.
 */
type SpendScope = "organization" | { user: string };

const CostSetupBadge: FC<{ scope: SpendScope }> = ({ scope }) => {
	const isOrganization = scope === "organization";
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge asChild variant="warning" size="sm" hover>
					<button
						type="button"
						aria-label={
							isOrganization
								? "Cost setup for total spend"
								: `Cost setup for ${scope.user}`
						}
						className="cursor-default font-medium"
					>
						<TriangleAlertIcon />
						Cost setup
					</button>
				</Badge>
			</TooltipTrigger>
			<TooltipContent side="bottom" className="max-w-xs">
				{isOrganization
					? "Some users have used models without configured pricing. That usage is excluded, so total spend may be higher than shown."
					: "This user has used models without configured pricing. That usage is excluded, so their actual spend may be higher than shown."}
			</TooltipContent>
		</Tooltip>
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
			{unpricedUsageCount > 0 && <CostSetupBadge scope={scope} />}
		</span>
	);
};
