import dayjs from "dayjs";
import type { GetLicensesResponse } from "#/api/api";
import type { Feature } from "#/api/typesGenerated";

/**
 * A license is applicable when past its nbf and not expired, or when the
 * feature is in its grace period (an expired license can still grant it).
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
