import type { Entitlements, Organization } from "#/api/typesGenerated";

// Top-level navigation must admit organization group member readers without
// site-wide AI settings permissions, and access follows the entitlement even
// while a disabled organizations query keeps its cached result.
export const canViewAISpend = (
	entitlements: Entitlements,
	spendOrganizations: readonly Organization[] | undefined,
): boolean =>
	entitlements.features.aibridge.enabled &&
	(spendOrganizations?.length ?? 0) > 0;
