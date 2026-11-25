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
					Some usage is from models without configured pricing. That usage is
					excluded, so the actual cost may be higher than shown.
				</TooltipMessage>
			</InfoTooltip>
		)}
	</span>
);
