import type {
	UserAISpendStatus,
	UserOrganizationAISpend,
} from "#/api/typesGenerated";
import { getSeverity, type UsageSeverity } from "#/utils/budget";

type SpendFields = Pick<
	UserOrganizationAISpend,
	"current_spend_micros" | "effective_budget"
>;

// Servers older than the per-organization breakdown omit `organizations`.
type SpendStatus = Omit<UserAISpendStatus, "organizations"> &
	Partial<Pick<UserAISpendStatus, "organizations">>;

/** AI spend and its limit, in micros, as the user menu shows them. */
export type ViewedSpend = {
	currentSpend: number;
	/** A null limit means unlimited. */
	spendLimit: number | null;
};

/**
 * Converts API spend fields for display. Returns null when the server sent a
 * negative value. A null budget is unlimited and still shown.
 */
export function toViewedSpend(fields: SpendFields): ViewedSpend | null {
	const limit = fields.effective_budget?.spend_limit_micros ?? null;
	if (fields.current_spend_micros < 0 || (limit !== null && limit < 0)) {
		return null;
	}
	return { currentSpend: fields.current_spend_micros, spendLimit: limit };
}

/**
 * Classifies spend against its limit. Missing or unlimited spend is normal. A
 * limit of 0 is exceeded even with no spend, because AI Gateway blocks requests
 * once spend reaches the limit.
 */
export function getSpendSeverity(spend: ViewedSpend | null): UsageSeverity {
	if (!spend || spend.spendLimit === null) {
		return "normal";
	}
	if (spend.spendLimit === 0) {
		return "exceeded";
	}
	return getSeverity(spend.currentSpend, spend.spendLimit);
}

/**
 * Picks the spend shown in the user menu. `shownOrgSpend` is set only for
 * users in more than one organization, even when the selection is gone after
 * a refetch. Everyone else, and servers that omit `organizations`, gets the
 * top-level fields.
 */
export function getViewedSpend(
	data: SpendStatus,
	selectedOrgId: string | undefined,
): { shownOrgSpend?: UserOrganizationAISpend; spend: ViewedSpend | null } {
	const organizations = data.organizations ?? [];
	if (organizations.length <= 1) {
		return { spend: toViewedSpend(data) };
	}
	const findOrg = (id: string | null | undefined) =>
		organizations.find((entry) => entry.organization.id === id);
	const shownOrgSpend =
		findOrg(selectedOrgId) ?? findOrg(data.organization_id) ?? organizations[0];
	return { shownOrgSpend, spend: toViewedSpend(shownOrgSpend) };
}
