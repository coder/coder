import type { FC } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { formatCostMicros } from "#/utils/currency";

const unpricedUsageWarnings = {
	user: {
		label: "Unpriced models",
		explanation:
			"This user has used models without configured pricing. That usage is excluded, so their actual spend may be higher than shown.",
	},
	organization: {
		label: "Unpriced models in total spend",
		explanation:
			"Some users have used models without configured pricing. That usage is excluded, so total spend may be higher than shown.",
	},
};

type SpendAmountProps = {
	costMicros: number;
	unpricedUsageCount: number;
	/** Whose unpriced usage the warning describes. */
	scope: keyof typeof unpricedUsageWarnings;
};

/** Shows spend as a lower bound when model pricing is missing. */
export const SpendAmount: FC<SpendAmountProps> = ({
	costMicros,
	unpricedUsageCount,
	scope,
}) => (
	<span className="inline-flex items-center gap-1 tabular-nums">
		{formatCostMicros(costMicros)}
		{unpricedUsageCount > 0 && (
			<InfoTooltip
				type="warning"
				size="small"
				ariaLabel={unpricedUsageWarnings[scope].label}
			>
				{unpricedUsageWarnings[scope].explanation}
			</InfoTooltip>
		)}
	</span>
);
