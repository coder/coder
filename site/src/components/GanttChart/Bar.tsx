import type { Interpolation, Theme } from "@emotion/react";
import { forwardRef, type HTMLProps, type ReactNode } from "react";

type BarColor = "default" | "green";

type BarProps = Omit<HTMLProps<HTMLDivElement>, "size"> & {
	width: number;
	children?: ReactNode;
	color?: BarColor;
	/**
	 * Label to be displayed adjacent to the bar component.
	 */
	afterLabel?: ReactNode;
	/**
	 * The X position of the bar component.
	 */
	x?: number;
};

export const Bar = forwardRef<HTMLDivElement, BarProps>(
	(
		{ color = "default", width, afterLabel, children, x, ...htmlProps },
		ref,
	) => {
		return (
			<div
				ref={ref}
				css={[styles.root, { transform: `translateX(${x}px)` }]}
				{...htmlProps}
			>
				<button
					type="button"
					css={[styles.bar, colorStyles[color], { width }]}
					disabled={htmlProps.disabled}
					aria-labelledby={htmlProps["aria-labelledby"]}
				>
					{children}
				</button>
				{afterLabel}
			</div>
		);
	},
);

const styles = {
	root: {
		// Stack children horizontally for adjacent labels
		display: "flex",
		alignItems: "center",
		width: "fit-content",
		gap: 8,
	},
	bar: {
		border: "1px solid transparent",
		borderRadius: 8,
		height: 32,
		display: "flex",
		padding: 0,

		"&:not(:disabled)": {
			cursor: "pointer",

			"&:focus, &:hover, &:active": {
				outline: "none",
				background: "#082F49",
				borderColor: "#38BDF8",
			},
		},
	},
} satisfies Record<string, Interpolation<Theme>>;

const colorStyles = {
	default: (theme) => ({
		backgroundColor: theme.palette.background.default,
		borderColor: theme.palette.divider,
	}),
	green: (theme) => ({
		backgroundColor: theme.roles.success.background,
		borderColor: theme.roles.success.outline,
		color: theme.roles.success.text,
	}),
} satisfies Record<BarColor, Interpolation<Theme>>;
