import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { Button } from "#/components/Button/Button";
import {
	PaywallFeature,
	PaywallFeatures,
	PREMIUM_FEATURES,
} from "#/components/Paywall/Paywall";
import { Skeleton } from "#/components/Skeleton/Skeleton";

export const LICENSES_PAGE_PATH = "/deployment/licenses";
export const CONTACT_SALES_LINK = "https://coder.com/contact/sales";

interface TrialActivePanelProps {
	/** Undefined when the trial license has no readable expiry. */
	daysRemaining: number | undefined;
	isLoading: boolean;
}

const remainingLabel = (daysRemaining: number | undefined): string => {
	if (daysRemaining === undefined) {
		return "Your Premium trial is active.";
	}
	if (daysRemaining < 0) {
		return "Your Premium trial has expired.";
	}
	if (daysRemaining === 0) {
		return "Your Premium trial ends today.";
	}
	return `${daysRemaining} ${daysRemaining === 1 ? "day" : "days"} remaining`;
};

export const TrialActivePanel: FC<TrialActivePanelProps> = ({
	daysRemaining,
	isLoading,
}) => {
	return (
		<div className="flex flex-col gap-6">
			<div className="flex flex-col gap-2">
				<h1 className="m-0 font-semibold text-2xl text-content-primary">
					Your Premium trial is active
				</h1>
				{isLoading ? (
					<Skeleton
						className="h-5 w-40"
						data-testid="trial-remaining-skeleton"
					/>
				) : (
					<p className="m-0 text-sm text-content-secondary">
						{remainingLabel(daysRemaining)}
					</p>
				)}
			</div>

			<PaywallFeatures className="px-0" aria-label="Premium features">
				{PREMIUM_FEATURES.map((feature) => (
					<PaywallFeature key={feature}>{feature}</PaywallFeature>
				))}
			</PaywallFeatures>

			<div className="flex flex-wrap gap-3">
				<Button asChild>
					<a href={CONTACT_SALES_LINK} target="_blank" rel="noreferrer">
						Contact sales
						<ExternalLinkIcon aria-hidden="true" className="size-icon-sm" />
					</a>
				</Button>
				<Button variant="outline" asChild>
					<RouterLink to={LICENSES_PAGE_PATH}>View licenses</RouterLink>
				</Button>
			</div>
		</div>
	);
};
