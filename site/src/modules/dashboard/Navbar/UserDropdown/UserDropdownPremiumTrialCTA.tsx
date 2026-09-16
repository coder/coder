import { BadgeCheckIcon, ClockIcon } from "lucide-react";
import type { FC } from "react";
import { useMutation, useQuery } from "react-query";
import { Link } from "react-router";
import { licenses } from "#/api/queries/licenses";
import { reportPremiumFunnelEvent } from "#/api/queries/premiumFunnel";
import { DropdownMenuItem } from "#/components/DropdownMenu/DropdownMenu";
import { PREMIUM_PAGE_PATH } from "#/components/Paywall/Paywall";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { trackPremiumFunnelClick } from "#/modules/paywall/premiumFunnelAttribution";

const TRIAL_LENGTH_DAYS = 30;
const TRIAL_LICENSE_STALE_TIME_MS = 15 * 60 * 1000;
const DAY_SECONDS = 24 * 60 * 60;

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
	/** Unix seconds of approximately right now */
	readonly now: number;
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

	if (trialExpiresAt < now) {
		return undefined;
	}

	const days = Math.floor((trialExpiresAt - now) / DAY_SECONDS);
	return days <= 0
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
 * Offers a premium trial, or counts down an active one. Renders under
 * DashboardProvider, and stays hidden unless the viewer can read licenses.
 */
export const UserDropdownPremiumTrialCTA: FC<
	UserDropdownPremiumTrialCTAProps
> = ({ canViewLicenses }) => {
	const { entitlements } = useDashboard();
	const { mutate: reportClick } = useMutation(reportPremiumFunnelEvent());

	const licensesQuery = useQuery({
		...licenses(),
		enabled: canViewLicenses && entitlements.has_license && entitlements.trial,
		staleTime: TRIAL_LICENSE_STALE_TIME_MS,
	});

	const cta = selectTrialCta({
		canViewLicenses,
		hasLicense: entitlements.has_license,
		isTrial: entitlements.trial,
		trialExpiresAt: licensesQuery.data?.find((license) => license.claims.trial)
			?.claims.license_expires,
		now: Math.floor(Date.now() / 1000),
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
