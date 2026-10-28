import { cn } from "cn";
import type React from "react";
import { Link as RouterLink } from "react-router";
import { Link, type LinkProps } from "#/components/Link/Link";
import { PREMIUM_PAGE_PATH } from "./Paywall";

type PremiumPageLinkProps = {
	size?: LinkProps["size"];
	className?: string;
};

/**
 * A text link to the Premium page. Render it only for users who can view
 * licenses; the Premium page is not useful to anyone else.
 */
export const PremiumPageLink: React.FC<PremiumPageLinkProps> = ({
	size = "sm",
	className,
}) => {
	return (
		<Link asChild size={size} showExternalIcon={false} className={className}>
			<RouterLink to={PREMIUM_PAGE_PATH}>Learn about Premium</RouterLink>
		</Link>
	);
};

type PremiumNoticeProps = React.ComponentProps<"span"> & {
	/** Shows the link to the Premium page. Pass `permissions.viewAllLicenses`. */
	canViewPremium: boolean;
};

/**
 * Explains why a single setting is disabled without a Premium license. Place
 * it under the setting's helper text and mark the setting with a
 * `PremiumBadge`. For a whole page, use a paywall instead.
 *
 * The badge stays a label. The link is a separate element so that it looks
 * interactive and is only shown to users who can act on it.
 */
export const PremiumNotice: React.FC<PremiumNoticeProps> = ({
	canViewPremium,
	className,
	children,
	...props
}) => {
	return (
		<span
			className={cn("block text-xs text-content-secondary", className)}
			{...props}
		>
			{children}
			{canViewPremium && (
				<>
					{" "}
					<PremiumPageLink />
				</>
			)}
		</span>
	);
};
