import { cn } from "cn";
import type React from "react";
import { Link as RouterLink } from "react-router";
import { PremiumBadge } from "#/components/Badge/PresetBadges";
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

type PremiumNoticeProps = React.ComponentProps<"div"> & {
	/** Shows the link to the Premium page. Pass `permissions.viewAllLicenses`. */
	canViewPremium: boolean;
};

/**
 * Marks a single setting that needs a Premium license. Use it next to a
 * disabled control. For a whole page or section, use a paywall instead.
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
		<div
			className={cn(
				"flex flex-row items-center gap-4 text-xs text-content-secondary",
				className,
			)}
			{...props}
		>
			<PremiumBadge />
			<span>
				{children}
				{canViewPremium && (
					<>
						{" "}
						<PremiumPageLink />
					</>
				)}
			</span>
		</div>
	);
};
