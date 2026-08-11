import type { ReactNode } from "react";
import { cn } from "#/utils/cn";
import {
	Paywall,
	PaywallContent,
	PaywallCTALink,
	PaywallDescription,
	PaywallDocumentationLink,
	PaywallFeature,
	PaywallFeatures,
	PaywallGuidance,
	PaywallHeading,
	PaywallSeparator,
	PaywallStack,
	PaywallSupergraphic,
	PaywallTitle,
} from "./Paywall";

const PREMIUM_FEATURES = [
	"High availability & workspace proxies",
	"Multi-org & role-based access control",
	"24x7 global support with SLA",
	"Unlimited Git & external auth integrations",
];

const PREMIUM_PAGE_PATH = "/deployment/premium";
const PREMIUM_PRICING_LINK = "https://coder.com/pricing";

type PaywallPremiumProps = React.ComponentProps<"div"> & {
	message: string;
	description: ReactNode;
	compact?: boolean;
	/** Whether the viewer can reach the in-app Premium page. */
	canViewPremium: boolean;
};

const PaywallPremium = ({
	message,
	description,
	compact = false,
	canViewPremium: canViewLicenses,
	className,
	...props
}: PaywallPremiumProps) => {
	return (
		<Paywall
			className={cn(
				compact && "max-w-[770px] py-4 px-[36px] gap-[18px] min-h-[230px]",
				className,
			)}
			{...props}
		>
			<PaywallSupergraphic />
			<PaywallContent>
				<PaywallHeading className={cn(compact && "mb-[18px]")}>
					<PaywallTitle className={cn(compact && "text-lg leading-none")}>
						{message}
					</PaywallTitle>
				</PaywallHeading>
				<PaywallDescription
					className={cn(
						compact &&
							"text-sm max-w-[360px] mt-2 mb-3.5 leading-relaxed text-content-secondary",
					)}
				>
					{description}
				</PaywallDescription>
				<PaywallDocumentationLink href={PREMIUM_PRICING_LINK}>
					Read the documentation
				</PaywallDocumentationLink>
			</PaywallContent>
			<PaywallSeparator className="h-[180px]" />
			<PaywallStack className={cn(compact && "gap-4")}>
				<PaywallFeatures className={cn(compact && "pr-0")}>
					{PREMIUM_FEATURES.map((feature) => (
						<PaywallFeature
							className={cn(compact && "text-[13px] leading-tight")}
							key={feature}
						>
							{feature}
						</PaywallFeature>
					))}
				</PaywallFeatures>
				{canViewLicenses ? (
					<PaywallCTALink to={PREMIUM_PAGE_PATH}>
						Learn about Premium
					</PaywallCTALink>
				) : (
					<PaywallGuidance>
						Contact your deployment administrator for Premium.
					</PaywallGuidance>
				)}
			</PaywallStack>
		</Paywall>
	);
};

export { PaywallPremium };
