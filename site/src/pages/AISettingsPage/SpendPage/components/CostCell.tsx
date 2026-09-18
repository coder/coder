import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { TableCell } from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatCostMicros } from "#/utils/currency";

const unpricedUsageExplanations = {
	user: "This user has used models without configured pricing. That usage is excluded, so their actual spend may be higher than shown.",
	organization:
		"Some users have used models without configured pricing. That usage is excluded, so total spend may be higher than shown.",
};

interface SpendAmountProps {
	costMicros: number;
	unpricedUsageCount: number;
	/** Whose unpriced usage the warning describes. */
	scope: keyof typeof unpricedUsageExplanations;
}

/** Shows spend as a lower bound when model pricing is missing. */
export const SpendAmount: FC<SpendAmountProps> = ({
	costMicros,
	unpricedUsageCount,
	scope,
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
					{unpricedUsageExplanations[scope]}
				</TooltipContent>
			</Tooltip>
		)}
	</span>
);

export const CostCell: FC<Omit<SpendAmountProps, "scope">> = (props) => (
	<TableCell className="text-right">
		<SpendAmount scope="user" {...props} />
	</TableCell>
);
