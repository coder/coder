import type { NetworkPolicyRevision } from "#/api/typesGenerated";

/** Returns the newest saved revision, or undefined when none have been saved. */
export const latestNetworkPolicyRevision = (
	revisions: readonly NetworkPolicyRevision[],
): NetworkPolicyRevision | undefined =>
	revisions.toSorted((a, b) => b.created_at.localeCompare(a.created_at))[0];
