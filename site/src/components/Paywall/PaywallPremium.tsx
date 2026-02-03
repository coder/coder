import { PremiumBadge } from "components/Badges/Badges";
import type { ReactNode } from "react";
import { cn } from "utils/cn";
import {
	Paywall,
	PaywallContent,
	PaywallCTA,
	PaywallDescription,
	PaywallDocumentationLink,
	PaywallFeature,
	PaywallFeatures,
	PaywallHeading,
	PaywallSeparator,
	PaywallStack,
	PaywallTitle,
} from "./Paywall";

type PaywallPremiumProps = React.ComponentProps<"div"> & {
	message: string;
	description: ReactNode;
	documentationLink: string;
	compact?: boolean;
};

const PaywallPremium = ({
	message,
	description,
	documentationLink,
	compact = false,
	className,
	...props
}: PaywallPremiumProps) => {
	const PREMIUM_FEATURES = [
		"High availability & workspace proxies",
		"Multi-org & role-based access control",
		"24x7 global support with SLA",
		"Unlimited Git & external auth integrations",
	];

	return (
		<Paywall
			className={cn(
				compact && "max-w-[770px] py-4 px-[36px] gap-[18px] min-h-[230px]",
				className,
			)}
			{...props}
		>
			<PaywallContent>
				<PaywallHeading className={cn(compact && "mb-[18px]")}>
					<PaywallTitle className={cn(compact && "text-lg leading-none")}>
						{message}
					</PaywallTitle>
					<PremiumBadge />
				</PaywallHeading>
				<PaywallDescription
					className={cn(
						compact &&
							"text-sm max-w-[360px] mt-2 mb-3.5 leading-relaxed text-content-secondary",
					)}
				>
					{description}
				</PaywallDescription>
				<PaywallDocumentationLink href={documentationLink}>
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
				<PaywallCTA href="https://coder.com/pricing#compare-plans">
					Learn about Premium
				</PaywallCTA>
			</PaywallStack>
		</Paywall>
	);
};

export { PaywallPremium };
