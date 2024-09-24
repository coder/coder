import type { Interpolation, Theme } from "@emotion/react";
import { forwardRef, type HTMLProps, type ReactNode } from "react";

export type BarColor = {
	border: string;
	fill: string;
};

type BarProps = Omit<HTMLProps<HTMLDivElement>, "size" | "color"> & {
	width: number;
	children?: ReactNode;
	/**
	 * Color scheme for the bar. If not passed the default gray color will be
	 * used.
	 */
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
	({ color, width, afterLabel, children, x, ...htmlProps }, ref) => {
		return (
			<div
				ref={ref}
				css={[styles.root, { transform: `translateX(${x}px)` }]}
				{...htmlProps}
			>
				<button
					type="button"
					css={[
						styles.bar,
						{ width, backgroundColor: color?.fill, borderColor: color?.border },
					]}
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
	bar: (theme) => ({
		border: "1px solid",
		borderColor: theme.palette.divider,
		backgroundColor: theme.palette.background.default,
		borderRadius: 8,
		height: 32,
		display: "flex",
		padding: 0,
		minWidth: 8,

		"&:not(:disabled)": {
			cursor: "pointer",

			"&:focus, &:hover, &:active": {
				outline: "none",
				borderColor: "#38BDF8",
			},
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
