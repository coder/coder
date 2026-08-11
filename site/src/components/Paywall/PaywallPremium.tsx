import { ExternalLinkIcon } from "lucide-react";
import { Supergraphic } from "#/components/Supergraphic/Supergraphic";
import { cn } from "#/utils/cn";
import {
	PaywallCTALink,
	PaywallFeature,
	PaywallFeatures,
	PaywallGuidance,
	type PaywallProps,
	PaywallTitle,
	PREMIUM_DEFAULT_DESCRIPTION,
	PREMIUM_DEFAULT_HERO,
	PREMIUM_FEATURES,
	PREMIUM_PAGE_PATH,
	PREMIUM_PRICING_LINK,
} from "./Paywall";

const DEFAULT_HERO_SUBTITLE = "Start a 30-day trial today.";

const PaywallPremium = ({
	message,
	description = PREMIUM_DEFAULT_DESCRIPTION,
	canViewPremium,
	className,
	features = PREMIUM_FEATURES,
	...props
}: PaywallProps) => {
	return (
		<div
			className={cn(
				"rounded-lg border border-solid border-border-default bg-surface-primary p-2",
				className,
			)}
			{...props}
		>
			<section className="relative isolate overflow-hidden rounded-lg flex flex-col items-center text-center py-12 px-6 mb-8">
				<Supergraphic className="absolute inset-0 -z-10" />
				<PaywallTitle>{PREMIUM_DEFAULT_HERO}</PaywallTitle>
				<p className="mt-3 mb-0 text-sm">{DEFAULT_HERO_SUBTITLE}</p>
				{canViewPremium ? (
					<PaywallCTALink to={PREMIUM_PAGE_PATH} className="mt-6 mx-0">
						Start trial for free
					</PaywallCTALink>
				) : (
					<PaywallGuidance className="mt-6 mx-0">
						Contact your deployment administrator for Premium.
					</PaywallGuidance>
				)}
			</section>

			<div className="grid grid-cols-1 md:grid-cols-2 gap-8 px-6 pb-6">
				<div>
					<h3 className="m-0 font-semibold text-base leading-relaxed text-content-primary">
						{description}
					</h3>
					<a
						href={PREMIUM_PRICING_LINK}
						target="_blank"
						rel="noreferrer"
						className="mt-4 inline-flex items-center gap-1.5 text-sm text-content-link underline underline-offset-2"
					>
						Learn more about premium
						<ExternalLinkIcon aria-hidden="true" className="size-icon-sm" />
					</a>
				</div>
				<PaywallFeatures className="px-0" aria-label={message}>
					{features.map((feature) => (
						<PaywallFeature key={feature}>{feature}</PaywallFeature>
					))}
				</PaywallFeatures>
			</div>
		</div>
	);
};

export { PaywallPremium };
