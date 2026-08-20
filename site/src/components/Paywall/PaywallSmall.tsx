import { cn } from "#/utils/cn";
import {
	Paywall,
	PaywallContent,
	PaywallCTALink,
	PaywallDescription,
	PaywallFeature,
	PaywallFeatures,
	PaywallGuidance,
	PaywallHeading,
	type PaywallProps,
	PaywallStack,
	PaywallSupergraphic,
	PaywallTitle,
	PREMIUM_DEFAULT_HERO,
	PREMIUM_FEATURES,
	PREMIUM_PAGE_PATH,
} from "./Paywall";

const PaywallSmall = ({
	description,
	compact = false,
	canViewPremium,
	className,
	features = PREMIUM_FEATURES,
	...props
}: PaywallProps) => {
	return (
		<Paywall
			className={cn(
				compact && "max-w-[770px] p-4 gap-[18px] min-h-[230px]",
				className,
			)}
			{...props}
		>
			<PaywallSupergraphic className="bg-[length:auto_140%] bg-[position:50%_50%]" />
			<PaywallContent className="ml-8 items-start text-left">
				<PaywallHeading className={cn(compact && "justify-start mb-[18px]")}>
					<PaywallTitle className={cn(compact && "text-lg leading-none")}>
						{PREMIUM_DEFAULT_HERO}
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
			</PaywallContent>
			<PaywallStack className={cn(compact && "gap-4")}>
				<PaywallFeatures className={cn(compact && "pr-0")}>
					{features.map((feature) => (
						<PaywallFeature
							className={cn(compact && "text-sm font-normal leading-tight")}
							key={feature}
						>
							{feature}
						</PaywallFeature>
					))}
				</PaywallFeatures>
				{canViewPremium ? (
					<PaywallCTALink to={PREMIUM_PAGE_PATH} className="w-full ml-0 mr-8">
						Start trial for free
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

export { PaywallSmall };
