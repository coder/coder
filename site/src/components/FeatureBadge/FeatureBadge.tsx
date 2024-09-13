import type { Interpolation, Theme } from "@emotion/react";
import Link from "@mui/material/Link";
import { visuallyHidden } from "@mui/utils";
import { HelpTooltipContent } from "components/HelpTooltip/HelpTooltip";
import { Popover, PopoverTrigger } from "components/Popover/Popover";
import {
	useEffect,
	useState,
	type FC,
	type HTMLAttributes,
	type ReactNode,
} from "react";
import tailwindColors from "theme/tailwindColors";
import { docs } from "utils/docs";

/**
 * All types of feature that we are currently supporting. Defined as record to
 * ensure that we can't accidentally make typos when writing the badge text.
 */
const featureBadgeTypes = {
	beta: "beta",
	experimental: "experimental",
} as const satisfies Record<string, ReactNode>;

const styles = {
	badge: (theme) => ({
		// Base type is based on a span so that the element can be placed inside
		// more types of HTML elements without creating invalid markdown, but we
		// still want the default display behavior to be div-like
		display: "block",
		maxWidth: "fit-content",

		// Base style assumes that small badges will be the default
		fontSize: "0.75rem",

		cursor: "default",
		flexShrink: 0,
		padding: "4px 8px",
		lineHeight: 1,
		whiteSpace: "nowrap",
		border: `1px solid ${theme.roles.preview.outline}`,
		color: theme.roles.preview.text,
		backgroundColor: theme.roles.preview.background,
		borderRadius: "6px",
		transition:
			"color 0.2s ease-in-out, border-color 0.2s ease-in-out, background-color 0.2s ease-in-out",
	}),

	badgeHover: (theme) => ({
		color: theme.roles.preview.hover.text,
		borderColor: theme.roles.preview.hover.outline,
		backgroundColor: theme.roles.preview.hover.background,
	}),

	badgeLargeText: {
		fontSize: "1rem",
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

type FeatureBadgeProps = Readonly<
	Omit<HTMLAttributes<HTMLSpanElement>, "children"> & {
		type: keyof typeof featureBadgeTypes;
		size?: "sm" | "lg";

		/**
		 * Defines how the FeatureBadge should render.
		 * - interactive (default) - The badge functions like a link and
		 *   controls its own hover styling.
		 * - static - The badge is completely static and has no interaction
		 *   behavior.
		 * - staticHover - The badge is completely static, but displays badge
		     hover styling (but nothing related to links). Useful if you want a
			 parent component to control the hover styling.
		 */
		variant?: "interactive" | "static" | "staticHover";
	}
>;

export const FeatureBadge: FC<FeatureBadgeProps> = ({
	type,
	size = "sm",
	variant = "interactive",
	onPointerEnter,
	onPointerLeave,
	...delegatedProps
}) => {
	// Not a big fan of having two hover variables, but we need to make sure the
	// badge maintains its hover styling while the mouse is inside the tooltip
	const [isBadgeHovering, setIsBadgeHovering] = useState(false);
	const [isTooltipHovering, setIsTooltipHovering] = useState(false);

	useEffect(() => {
		const onWindowBlur = () => {
			setIsBadgeHovering(false);
			setIsTooltipHovering(false);
		};

		window.addEventListener("blur", onWindowBlur);
		return () => window.removeEventListener("blur", onWindowBlur);
	}, []);

	const featureType = featureBadgeTypes[type];
	const showBadgeHoverStyle =
		variant === "staticHover" ||
		(variant === "interactive" && (isBadgeHovering || isTooltipHovering));

	const coreContent = (
		<span
			css={[
				styles.badge,
				size === "lg" && styles.badgeLargeText,
				showBadgeHoverStyle && styles.badgeHover,
			]}
			onPointerEnter={variant === "interactive" ? undefined : onPointerEnter}
			onPointerLeave={variant === "interactive" ? undefined : onPointerLeave}
			{...delegatedProps}
		>
			<span style={visuallyHidden}> (This is a</span>
			{featureType}
			<span style={visuallyHidden}> feature)</span>
		</span>
	);

	if (variant !== "interactive") {
		return coreContent;
	}

	return (
		<Popover mode="hover">
			<PopoverTrigger
				onPointerEnter={(event) => {
					setIsBadgeHovering(true);
					onPointerEnter?.(event);
				}}
				onPointerLeave={(event) => {
					setIsBadgeHovering(false);
					onPointerLeave?.(event);
				}}
			>
				{coreContent}
			</PopoverTrigger>

			<HelpTooltipContent
				anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
				transformOrigin={{ vertical: "top", horizontal: "center" }}
				onPointerEnter={() => setIsTooltipHovering(true)}
				onPointerLeave={() => setIsTooltipHovering(false)}
			>
				<h5 css={styles.tooltipTitle}>
					{capitalizeFirstLetter(featureType)} Feature
				</h5>

				<p css={styles.tooltipDescription}>
					This is {getGrammaticalArticle(featureType)} {featureType} feature. It
					has not yet reached generally availability (GA).
				</p>

				<Link
					href={docs("/contributing/feature-stages")}
					target="_blank"
					rel="noreferrer"
					css={styles.tooltipLink}
				>
					Learn about feature stages
					<span style={visuallyHidden}> (link opens in new tab)</span>
				</Link>
			</HelpTooltipContent>
		</Popover>
	);
};

function getGrammaticalArticle(nextWord: string): string {
	const vowels = ["a", "e", "i", "o", "u"];
	const firstLetter = nextWord.slice(0, 1).toLowerCase();
	return vowels.includes(firstLetter) ? "an" : "a";
}

function capitalizeFirstLetter(text: string): string {
	return text.slice(0, 1).toUpperCase() + text.slice(1);
}
