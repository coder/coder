import type { Interpolation, Theme } from "@emotion/react";
import Link from "@mui/material/Link";
import { visuallyHidden } from "@mui/utils";
import { HelpTooltipContent } from "components/HelpTooltip/HelpTooltip";
import { Popover, PopoverTrigger } from "components/deprecated/Popover/Popover";
import type { FC, HTMLAttributes, ReactNode } from "react";
import { docs } from "utils/docs";

/**
 * All types of feature that we are currently supporting. Defined as record to
 * ensure that we can't accidentally make typos when writing the badge text.
 */
export const featureStageBadgeTypes = {
	beta: "beta",
	experimental: "experimental",
} as const satisfies Record<string, ReactNode>;

type FeatureStageBadgeProps = Readonly<
	Omit<HTMLAttributes<HTMLSpanElement>, "children"> & {
		contentType: keyof typeof featureStageBadgeTypes;
		size?: "sm" | "md" | "lg";
		showTooltip?: boolean;
	}
>;

export const FeatureStageBadge: FC<FeatureStageBadgeProps> = ({
	contentType,
	size = "md",
	showTooltip = true, // This is a temporary until the deprecated popover is removed
	...delegatedProps
}) => {
	return (
		<Popover mode="hover">
			<PopoverTrigger>
				{({ isOpen }) => (
					<span
						css={[
							styles.badge,
							size === "sm" && styles.badgeSmallText,
							size === "lg" && styles.badgeLargeText,
							isOpen && styles.badgeHover,
						]}
						{...delegatedProps}
					>
						<span style={visuallyHidden}> (This is a</span>
						<span css={styles.badgeLabel}>
							{featureStageBadgeTypes[contentType]}
						</span>
						<span style={visuallyHidden}> feature)</span>
					</span>
				)}
			</PopoverTrigger>

			{showTooltip && (
				<HelpTooltipContent
					anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
					transformOrigin={{ vertical: "top", horizontal: "center" }}
				>
					<p css={styles.tooltipDescription}>
						This feature has not yet reached general availability (GA).
					</p>

					<Link
						href={docs("/install/feature-stages")}
						target="_blank"
						rel="noreferrer"
						css={styles.tooltipLink}
					>
						Learn about feature stages
						<span style={visuallyHidden}> (link opens in new tab)</span>
					</Link>
				</HelpTooltipContent>
			)}
		</Popover>
	);
};

const styles = {
	badge: (theme) => ({
		// Base type is based on a span so that the element can be placed inside
		// more types of HTML elements without creating invalid markdown, but we
		// still want the default display behavior to be div-like
		display: "block",
		maxWidth: "fit-content",

		// Base style assumes that medium badges will be the default
		fontSize: "0.75rem",

		cursor: "default",
		flexShrink: 0,
		padding: "4px 8px",
		lineHeight: 1,
		whiteSpace: "nowrap",
		border: `1px solid ${theme.branding.featureStage.border}`,
		color: theme.branding.featureStage.text,
		backgroundColor: theme.branding.featureStage.background,
		borderRadius: "6px",
		transition:
			"color 0.2s ease-in-out, border-color 0.2s ease-in-out, background-color 0.2s ease-in-out",
	}),

	badgeHover: (theme) => ({
		color: theme.branding.featureStage.hover.text,
		borderColor: theme.branding.featureStage.hover.border,
		backgroundColor: theme.branding.featureStage.hover.background,
	}),

	badgeLabel: {
		// Have to set display mode to anything other than inline, or else the
		// CSS capitalization algorithm won't capitalize the element
		display: "inline-block",
		textTransform: "capitalize",
	},

	badgeLargeText: {
		fontSize: "1rem",
	},

	badgeSmallText: {
		// Have to beef up font weight so that the letters still maintain the
		// same relative thickness as all our other main UI text
		fontWeight: 500,
		fontSize: "0.625rem",
	},

	tooltipTitle: (theme) => ({
		color: theme.palette.text.primary,
		fontWeight: 600,
		fontFamily: "inherit",
		fontSize: 18,
		margin: 0,
		lineHeight: 1,
		paddingBottom: "8px",
	}),

	tooltipDescription: {
		margin: 0,
		lineHeight: 1.4,
		paddingBottom: "8px",
	},

	tooltipLink: {
		fontWeight: 600,
		lineHeight: 1.2,
	},
} as const satisfies Record<string, Interpolation<Theme>>;
