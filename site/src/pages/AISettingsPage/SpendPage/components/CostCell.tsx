import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { TableCell } from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatCostMicros } from "#/utils/currency";

interface SpendAmountProps {
	costMicros: number;
	unpricedUsageCount: number;
}

/** Shows spend as a lower bound when model pricing is missing. */
export const SpendAmount: FC<SpendAmountProps> = ({
	costMicros,
	unpricedUsageCount,
}) => (
	<span className="inline-flex items-center gap-1 tabular-nums">
		{formatCostMicros(costMicros)}
		{unpricedUsageCount > 0 && (
			<Tooltip>
				<TooltipTrigger
					type="button"
					aria-label="Unpriced models"
					className="inline-flex shrink-0 rounded-sm border-none bg-transparent p-0 text-content-warning focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-content-link"
				>
					<TriangleAlertIcon aria-hidden className="size-icon-xs" />
				</TooltipTrigger>
				<TooltipContent className="max-w-64">
					Usage from models without configured pricing is not included in this
					spend.
				</TooltipContent>
			</Tooltip>
		)}
	</span>
);

export const CostCell: FC<SpendAmountProps> = (props) => (
	<TableCell className="text-right">
		<SpendAmount {...props} />
	</TableCell>
);
