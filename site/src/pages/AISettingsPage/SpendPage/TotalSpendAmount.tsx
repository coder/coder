import { TriangleAlertIcon } from "lucide-react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatCostMicros } from "#/utils/currency";

type TotalSpendAmountProps = {
	costMicros: number;
	hasUnpricedUsage: boolean;
};

/** Shows total spend as a lower bound when model pricing is missing. */
export const TotalSpendAmount: React.FC<TotalSpendAmountProps> = ({
	costMicros,
	hasUnpricedUsage,
}) => (
	<span className="inline-flex items-center gap-2 tabular-nums">
		{formatCostMicros(costMicros)}
		{hasUnpricedUsage && (
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
		)}
	</span>
);
