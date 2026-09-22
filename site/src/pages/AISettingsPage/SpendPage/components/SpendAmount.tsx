import type { FC } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { formatCostMicros } from "#/utils/currency";

/**
 * Whose unpriced usage the warning describes. Each user row names its user
 * so the warning buttons stay distinguishable by accessible name.
 */
type SpendScope = "organization" | { user: string };

const UnpricedWarning: FC<{ scope: SpendScope }> = ({ scope }) => {
	if (scope === "organization") {
		return (
			<InfoTooltip
				type="warning"
				size="small"
				ariaLabel="Unpriced models in total spend"
			>
				Some users have used models without configured pricing. That usage is
				excluded, so total spend may be higher than shown.
			</InfoTooltip>
		);
	}
	return (
		<InfoTooltip
			type="warning"
			size="small"
			ariaLabel={`Unpriced models for ${scope.user}`}
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
		<span className="inline-flex items-center gap-1 tabular-nums">
			{unpricedUsageCount > 0 && <UnpricedWarning scope={scope} />}
			{formatCostMicros(costMicros)}
		</span>
	);
};
