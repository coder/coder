import type { Interpolation, Theme } from "@emotion/react";
import { visuallyHidden } from "@mui/utils";
import type { FC, HTMLAttributes, ReactNode } from "react";

/**
 * All types of feature that we are currently supporting. Defined as record to
 * ensure that we can't accidentally make typos when writing the badge text.
 */
const featureBadgeTypes = {
	beta: "beta",
	earlyAccess: "early access",
} as const satisfies Record<string, ReactNode>;

const styles = {
	badge: (theme) => ({
		// Base type is based on a span so that the element can be placed inside
		// more types of HTML elements without creating invalid markdown, but we
		// still want the default display behavior to be div-like
		display: "block",
		maxWidth: "fit-content",
		flexShrink: 0,
		padding: "8px 4px",
		border: `1px solid ${theme.palette.divider}`,
		color: theme.palette.text.secondary,
		backgroundColor: theme.palette.background.default,
		borderRadius: "6px",

		// Base style assumes that small badges will be the default
		fontSize: "0.75rem",
	}),

	highlighted: (theme) => ({
		color: theme.palette.text.primary,
		borderColor: theme.palette.text.primary,
	}),

	mediumText: {
		fontSize: "1rem",
	},
} as const satisfies Record<string, Interpolation<Theme>>;

type FeatureBadgeProps = Readonly<
	Omit<HTMLAttributes<HTMLSpanElement>, "children"> & {
		type: keyof typeof featureBadgeTypes;
		size?: "sm" | "md";
		highlighted?: boolean;
	}
>;

export const FeatureBadge: FC<FeatureBadgeProps> = ({
	type,
	size = "sm",
	highlighted = false,
	...delegatedProps
}) => {
	return (
		<span
			css={[
				styles.badge,
				size === "md" && styles.mediumText,
				highlighted && styles.highlighted,
			]}
			{...delegatedProps}
		>
			<span style={visuallyHidden}> (This feature is</span>
			{featureBadgeTypes[type]}
			<span style={visuallyHidden}>)</span>
		</span>
	);
};
