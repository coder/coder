import type { FC } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { formatCostMicros } from "#/utils/currency";

type SpendAmountProps = {
	costMicros: number;
	unpricedUsageCount: number;
	/**
	 * Whose unpriced usage the warning describes. Each user row names its
	 * user so the warning buttons stay distinguishable by accessible name.
	 */
	scope: "organization" | { user: string };
};

/** Shows spend as a lower bound when model pricing is missing. */
export const SpendAmount: FC<SpendAmountProps> = ({
	costMicros,
	unpricedUsageCount,
	scope,
}) => {
	const warning =
		scope === "organization"
			? {
					label: "Unpriced models in total spend",
					explanation:
						"Some users have used models without configured pricing. That usage is excluded, so total spend may be higher than shown.",
				}
			: {
					label: `Unpriced models for ${scope.user}`,
					explanation:
						"This user has used models without configured pricing. That usage is excluded, so their actual spend may be higher than shown.",
				};
	return (
		<span className="inline-flex items-center gap-1 tabular-nums">
			{formatCostMicros(costMicros)}
			{unpricedUsageCount > 0 && (
				<InfoTooltip type="warning" size="small" ariaLabel={warning.label}>
					{warning.explanation}
				</InfoTooltip>
			)}
		</span>
	);
};
