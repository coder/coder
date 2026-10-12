import type { Entitlements, Organization } from "#/api/typesGenerated";

export const canViewAISpend = (
	entitlements: Entitlements,
	spendOrganizations: readonly Organization[] | undefined,
): boolean =>
	entitlements.features.aibridge.enabled &&
	(spendOrganizations?.length ?? 0) > 0;
