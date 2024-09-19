import type { Interpolation, Theme } from "@emotion/react";
import type { FC, HTMLAttributes } from "react";

type LabelColor = "inherit" | "primary" | "secondary";

type LabelProps = HTMLAttributes<HTMLSpanElement> & {
	color?: LabelColor;
};

export const Label: FC<LabelProps> = ({ color = "inherit", ...htmlProps }) => {
	return <span {...htmlProps} css={[styles.label, colorStyles[color]]} />;
};

const styles = {
	label: {
		lineHeight: 1,
		fontSize: 12,
		fontWeight: 500,
		display: "inline-flex",
		alignItems: "center",
		gap: 4,

		"& svg": {
			fontSize: 12,
		},
	},
} satisfies Record<string, Interpolation<Theme>>;

const colorStyles = {
	inherit: {
		color: "inherit",
	},
	primary: (theme) => ({
		color: theme.palette.text.primary,
	}),
	secondary: (theme) => ({
		color: theme.palette.text.secondary,
	}),
} satisfies Record<LabelColor, Interpolation<Theme>>;
