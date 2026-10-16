import { Badge } from "#/components/Badge/Badge";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { TooltipMessage } from "#/components/Tooltip/Tooltip";
import { formatCostMicros } from "#/utils/currency";

type CostBadgeProps = {
	costMicros: number;
	hasUnpricedUsage: boolean;
};

export const CostBadge: React.FC<CostBadgeProps> = ({
	costMicros,
	hasUnpricedUsage,
}) => (
	<span className="inline-flex items-center gap-1 whitespace-nowrap">
		<Badge>{formatCostMicros(costMicros)}</Badge>
		{hasUnpricedUsage && (
			<InfoTooltip type="warning" size="small" ariaLabel="Unpriced usage">
				<TooltipMessage>
					Some usage has no cost, either because the model has no configured
					price or because it was recorded before cost tracking. The actual cost
					may be higher.
				</TooltipMessage>
			</InfoTooltip>
		)}
	</span>
);
