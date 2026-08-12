import dayjs from "dayjs";
import type { GetLicensesResponse } from "#/api/api";
import type { Feature } from "#/api/typesGenerated";

/**
 * Usage and overage indicators only apply to licenses that are currently
 * effective: past their nbf and not expired, unless the merged entitlement
 * for the feature is in its grace period (an expired license can still be
 * the one granting the feature while the grace period lasts).
 */
export function isLicenseApplicableForFeatureUsage(
	license: GetLicensesResponse,
	feature: Feature | undefined,
): boolean {
	const isExpired = dayjs
		.unix(license.claims.license_expires)
		.isBefore(dayjs());
	const isNotYetValid =
		license.claims.nbf !== undefined &&
		dayjs.unix(license.claims.nbf).isAfter(dayjs());
	const isFeatureInGracePeriod = feature?.entitlement === "grace_period";

	return !isNotYetValid && (!isExpired || isFeatureInGracePeriod);
}
