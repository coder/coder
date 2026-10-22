import type { NetworkPolicyTarget } from "#/api/typesGenerated";

/** An unknown outcome retains its server reservation until an operator reconciles it. */
export const isNetworkPolicyTargetBlocked = (
	target: NetworkPolicyTarget,
): boolean =>
	target.status === "pending" ||
	target.status === "running" ||
	target.status === "unknown";
