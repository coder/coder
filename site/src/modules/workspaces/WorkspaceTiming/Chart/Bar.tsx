import type { Interpolation, Theme } from "@emotion/react";
import { type ButtonHTMLAttributes, forwardRef, type HTMLProps } from "react";
import { CSSVars } from "./constants";

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
				css={barCss({ colors, scale, value, offset })}
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
				css={[...barCss({ colors, scale, value, offset }), styles.clickable]}
				{...htmlProps}
				ref={ref}
			/>
		);
	},
);

export const barCss = ({
	scale,
	value,
	colors,
	offset,
}: BaseBarProps<unknown>) => {
	return [
		styles.bar,
		{
			width: `calc((var(${CSSVars.xAxisWidth}) * ${value}) / ${scale})`,
			backgroundColor: colors?.fill,
			borderColor: colors?.stroke,
			marginLeft: `calc((var(${CSSVars.xAxisWidth}) * ${offset}) / ${scale})`,
		},
	];
};

const styles = {
	bar: (theme) => ({
		border: "1px solid",
		borderColor: theme.palette.divider,
		backgroundColor: theme.palette.background.default,
		borderRadius: 8,
		height: 32,
		display: "flex",
		padding: 0,
		minWidth: 8,
	}),
	clickable: {
		cursor: "pointer",

		"&:focus, &:hover, &:active": {
			outline: "none",
			borderColor: "#38BDF8",
		},
	},
} satisfies Record<string, Interpolation<Theme>>;
