import type { FC } from "react";
import type { To } from "react-router";
import { formatCostMicros } from "#/utils/currency";
import { UnpricedModelsWarning } from "./UnpricedModelsWarning";

type SpendAmountProps = {
	costMicros: number;
	unpricedUsageCount: number;
	/** Names the user so each row's warning has a distinct accessible name. */
	user: string;
	unpricedModels: readonly string[] | undefined;
	setPricingHref: To | undefined;
};

/**
 * Shows a user's spend, flagged when it excludes usage of models without
 * pricing. The icon sits before the amount so right-aligned figures stay
 * lined up.
 */
export const SpendAmount: FC<SpendAmountProps> = ({
	costMicros,
	unpricedUsageCount,
	user,
	unpricedModels,
	setPricingHref,
}) => (
	<span className="inline-flex items-center gap-2 tabular-nums">
		{unpricedUsageCount > 0 && (
			<UnpricedModelsWarning
				label={`Model pricing missing for ${user}`}
				models={unpricedModels}
				setPricingHref={setPricingHref}
				// Spend cells are right-aligned, so the card hangs off the table edge.
				align="end"
			/>
		)}
		{formatCostMicros(costMicros)}
	</span>
);
