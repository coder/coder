import type {
	AISpendPeriodWindow,
	UserAISpendStatus,
	UserOrganizationAISpend,
	UserOrganizationsAISpendStatus,
} from "#/api/typesGenerated";
import { getSeverity, type UsageSeverity } from "#/utils/budget";

type SpendFields = Pick<
	UserOrganizationAISpend,
	"current_spend_micros" | "effective_budget"
>;

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

/** Classifies spend against its limit. Missing or unlimited spend is normal. */
export function getSpendSeverity(spend: ViewedSpend | null): UsageSeverity {
	if (!spend || spend.spendLimit === null) {
		return "normal";
	}
	return getSeverity(spend.currentSpend, spend.spendLimit);
}

/** The spend the user menu shows, and the period it covers. */
type UserMenuSpend = {
	/** Set only for users in more than one organization. */
	orgSelection?: {
		organizations: readonly UserOrganizationAISpend[];
		shown: UserOrganizationAISpend;
	};
	/** Null when the server sent a negative spend or limit. */
	spend: ViewedSpend | null;
	period: AISpendPeriodWindow;
};

/**
 * Users in more than one organization see the selected organization's spend,
 * or else the `organization_id` entry, or else the first entry. Everyone else
 * sees the site-wide spend.
 */
export function getUserMenuSpend(
	siteWide: UserAISpendStatus | undefined,
	byOrganization: UserOrganizationsAISpendStatus | undefined,
	selectedOrgId: string | undefined,
): UserMenuSpend | undefined {
	if (byOrganization && byOrganization.organizations.length > 1) {
		const { organizations } = byOrganization;
		const findOrg = (id: string | null | undefined) =>
			organizations.find((entry) => entry.organization.id === id);
		const shown =
			findOrg(selectedOrgId) ??
			findOrg(byOrganization.organization_id) ??
			organizations[0];
		return {
			orgSelection: { organizations, shown },
			spend: toViewedSpend(shown),
			period: byOrganization,
		};
	}
	if (!siteWide) {
		return undefined;
	}
	return { spend: toViewedSpend(siteWide), period: siteWide };
}
