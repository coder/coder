import type { Interpolation, Theme } from "@emotion/react";
import { type ButtonHTMLAttributes, forwardRef, type HTMLProps } from "react";

export type BarColors = {
	stroke: string;
	fill: string;
};

type BaseBarProps<T> = Omit<T, "size" | "color"> & {
	/**
	 * The width of the bar component.
	 */
	size: number;
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
	({ colors, size, children, offset, ...htmlProps }, ref) => {
		return (
			<div css={barCss({ colors, size, offset })} {...htmlProps} ref={ref} />
		);
	},
);

type ClickableBarProps = BaseBarProps<ButtonHTMLAttributes<HTMLButtonElement>>;

export const ClickableBar = forwardRef<HTMLButtonElement, ClickableBarProps>(
	({ colors, size, offset, ...htmlProps }, ref) => {
		return (
			<button
				type="button"
				css={[...barCss({ colors, size, offset }), styles.clickable]}
				{...htmlProps}
				ref={ref}
			/>
		);
	},
);

export const barCss = ({ size, colors, offset }: BaseBarProps<unknown>) => {
	return [
		styles.bar,
		{
			width: size,
			backgroundColor: colors?.fill,
			borderColor: colors?.stroke,
			marginLeft: offset,
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
