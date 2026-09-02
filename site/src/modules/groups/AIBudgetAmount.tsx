import type { FC } from "react";
import { getSeverity, type UsageSeverity } from "#/utils/budget";
import { formatBudgetUSD } from "#/utils/currency";

const severityTextClasses = {
	normal: "text-content-primary",
	warning: "text-content-warning",
	exceeded: "text-content-destructive",
} as const satisfies Record<UsageSeverity, string>;

/** A spend amount in USD that takes the warning/exceeded color as it nears the limit; values in micros. */
export const AIBudgetAmount: FC<{ spend: number; limit: number }> = ({
	spend,
	limit,
}) => (
	<span className={severityTextClasses[getSeverity(spend, limit)]}>
		{formatBudgetUSD(spend)}
	</span>
);
