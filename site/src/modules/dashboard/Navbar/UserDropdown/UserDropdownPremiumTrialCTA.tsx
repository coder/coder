import dayjs, { type Dayjs } from "dayjs";
import { BadgeCheckIcon, ClockIcon } from "lucide-react";
import { type FC, useContext } from "react";
import { useMutation, useQuery } from "react-query";
import { Link } from "react-router";
import { licenses } from "#/api/queries/licenses";
import { reportPremiumFunnelEvent } from "#/api/queries/premiumFunnel";
import { DropdownMenuItem } from "#/components/DropdownMenu/DropdownMenu";
import { PREMIUM_PAGE_PATH } from "#/components/Paywall/Paywall";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import { trackPremiumFunnelClick } from "#/modules/paywall/premiumFunnelAttribution";

const TRIAL_LENGTH_DAYS = 30;
const TRIAL_LICENSE_STALE_TIME_MS = 15 * 60 * 1000;

type TrialCta = {
	readonly kind: "daysLeft" | "start" | "expiresToday";
	readonly days: number;
};

type TrialCtaInput = {
	readonly canViewLicenses: boolean;
	readonly hasLicense: boolean;
	readonly isTrial: boolean;
	/** Unix seconds from the trial license's license_expires claim. */
	readonly trialExpiresAt: number | undefined;
	readonly now: Dayjs;
};

export const selectTrialCta = ({
	canViewLicenses,
	hasLicense,
	isTrial,
	trialExpiresAt,
	now,
}: TrialCtaInput): TrialCta | undefined => {
	if (!canViewLicenses) {
		return undefined;
	}
	if (!hasLicense) {
		return { kind: "start", days: TRIAL_LENGTH_DAYS };
	}
	if (
		!isTrial ||
		trialExpiresAt === undefined ||
		!Number.isFinite(trialExpiresAt)
	) {
		return undefined;
	}

	const expiry = dayjs.unix(trialExpiresAt);
	if (!expiry.isAfter(now)) {
		return undefined;
	}

	const days = expiry.diff(now, "day");
	return days === 0
		? { kind: "expiresToday", days: 0 }
		: { kind: "daysLeft", days };
};

const trialCtaLabel = (cta: TrialCta): string => {
	switch (cta.kind) {
		case "start":
			return `Try premium for ${TRIAL_LENGTH_DAYS} days`;
		case "expiresToday":
			return "Trial expires today";
		case "daysLeft":
			return `${cta.days} ${cta.days === 1 ? "day" : "days"} left in trial`;
	}
};

interface UserDropdownPremiumTrialCTAProps {
	canViewLicenses: boolean;
}

/**
 * Offers a premium trial, or counts down an active one. Renders in the navbar
 * on every page, so it reads the dashboard defensively and degrades if missing data
 */
export const UserDropdownPremiumTrialCTA: FC<
	UserDropdownPremiumTrialCTAProps
> = ({ canViewLicenses }) => {
	const dashboard = useContext(DashboardContext);
	const { mutate: reportClick } = useMutation(reportPremiumFunnelEvent());
	const entitlements = dashboard?.entitlements;

	const licensesQuery = useQuery({
		...licenses(),
		enabled:
			canViewLicenses &&
			Boolean(entitlements?.has_license && entitlements?.trial),
		staleTime: TRIAL_LICENSE_STALE_TIME_MS,
	});

	if (!entitlements) {
		return null;
	}

	const cta = selectTrialCta({
		canViewLicenses,
		hasLicense: entitlements.has_license,
		isTrial: entitlements.trial,
		trialExpiresAt: licensesQuery.data?.find((license) => license.claims.trial)
			?.claims.license_expires,
		now: dayjs(),
	});
	if (!cta) {
		return null;
	}

	const ctaOnclick =
		cta.kind === "start"
			? () => reportClick(trackPremiumFunnelClick("direct", "small"))
			: undefined;
	return (
		<DropdownMenuItem asChild>
			<Link to={PREMIUM_PAGE_PATH} onClick={ctaOnclick}>
				{cta.kind === "start" ? <BadgeCheckIcon /> : <ClockIcon />}
				<span>{trialCtaLabel(cta)}</span>
			</Link>
		</DropdownMenuItem>
	);
};
