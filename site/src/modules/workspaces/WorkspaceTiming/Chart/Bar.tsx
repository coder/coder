import type { Interpolation, Theme } from "@emotion/react";
import { type ButtonHTMLAttributes, type HTMLProps, forwardRef } from "react";

export type BarColors = {
	stroke: string;
	fill: string;
};

type BaseBarProps<T> = Omit<T, "size" | "color"> & {
	/**
	 * Scale used to determine the width based on the given value.
	 */
	scale: number;
	value: number;
	/**
	 * The X position of the bar component.
	 */
	offset: number;
	/**
	 * Color scheme for the bar. If not passed the default gray color will be
	 * used.
	 */
	colors?: BarColors;
};

type BarProps = BaseBarProps<HTMLProps<HTMLDivElement>>;

export const Bar = forwardRef<HTMLDivElement, BarProps>(
	({ colors, scale, value, offset, ...htmlProps }, ref) => {
		return (
			<div
				css={barCSS({ colors, scale, value, offset })}
				{...htmlProps}
				ref={ref}
			/>
		);
	},
);

type ClickableBarProps = BaseBarProps<ButtonHTMLAttributes<HTMLButtonElement>>;

export const ClickableBar = forwardRef<HTMLButtonElement, ClickableBarProps>(
	({ colors, scale, value, offset, ...htmlProps }, ref) => {
		return (
			<button
				type="button"
				css={[...barCSS({ colors, scale, value, offset }), styles.clickable]}
				{...htmlProps}
				ref={ref}
			/>
		);
	},
);

export const barCSS = ({
	scale,
	value,
	colors,
	offset,
}: BaseBarProps<unknown>) => {
	return [
		styles.bar,
		{
			width: `calc((var(--x-axis-width) * ${value}) / ${scale})`,
			backgroundColor: colors?.fill,
			borderColor: colors?.stroke,
			marginLeft: `calc((var(--x-axis-width) * ${offset}) / ${scale})`,
		},
	];
};

const styles = {
	bar: (theme) => ({
		border: "1px solid",
		borderColor: theme.palette.divider,
		backgroundColor: theme.palette.background.default,
		borderRadius: 8,
		// The bar should fill the row height.
		height: "inherit",
		display: "flex",
		padding: 7,
		minWidth: 24,
		// Increase hover area
		position: "relative",
		"&::after": {
			content: '""',
			position: "absolute",
			top: -2,
			right: -8,
			bottom: -2,
			left: -8,
		},
	}),
	clickable: (theme) => ({
		cursor: "pointer",
		// We need to make the bar width at least 34px to allow the "..." icons to be displayed.
		// The calculation is border * 1 + side paddings * 2 + icon width (which is 18px)
		minWidth: 34,

		"&:focus, &:hover, &:active": {
			outline: "none",
			borderColor: theme.roles.active.outline,
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
