import type { OrganizationAISpendOverview } from "#/api/typesGenerated";
import { formatCostMicros } from "#/utils/currency";

/** The number of users the overview requests for the top spenders card. */
export const TOP_SPENDERS_COUNT = 10;

/**
 * Describes how much of the total the listed users account for. Returns
 * undefined when every user with spend is listed.
 */
export const summarizeTopSpenders = ({
	count,
	totals,
	users,
}: Pick<OrganizationAISpendOverview, "count" | "totals" | "users">):
	| string
	| undefined => {
	const otherCount = count - users.length;
	if (otherCount <= 0 || totals.spend_micros <= 0) {
		return undefined;
	}
	const topSpendMicros = users.reduce(
		(sum, user) => sum + user.spend_micros,
		0,
	);
	const percent = Math.round((topSpendMicros / totals.spend_micros) * 100);
	const top = users.length === 1 ? "The top user" : `Top ${users.length} users`;
	const others =
		otherCount === 1 ? "The other user" : `The other ${otherCount} users`;
	return `${top} spent ${percent}% of the total. ${others} spent ${formatCostMicros(totals.spend_micros - topSpendMicros)}.`;
};
