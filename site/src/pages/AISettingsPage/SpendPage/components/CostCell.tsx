import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { TableCell } from "#/components/Table/Table";
import { formatCostMicros } from "#/utils/currency";

interface CostCellProps {
	costMicros: number;
	unpricedUsageCount: number;
}

// A cost with unpriced usage is a lower bound, so the cell says so.
export const CostCell: FC<CostCellProps> = ({
	costMicros,
	unpricedUsageCount,
}) => (
	<TableCell className="text-right tabular-nums">
		{formatCostMicros(costMicros)}
		{unpricedUsageCount > 0 && (
			<span className="mt-0.5 flex items-center justify-end gap-1 text-content-warning">
				<TriangleAlertIcon aria-hidden className="size-icon-xs" />
				Unpriced usage
			</span>
		)}
	</TableCell>
);
