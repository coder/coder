import type { Entitlements, Organization } from "#/api/typesGenerated";

/**
 * Whether the viewer can open the AI spend page: AI Gateway is enabled and they
 * can read the spend of at least one organization.
 */
export const canViewAISpend = (
	entitlements: Entitlements,
	spendOrganizations: readonly Organization[] | undefined,
): boolean =>
	entitlements.features.aibridge.enabled &&
	(spendOrganizations?.length ?? 0) > 0;
