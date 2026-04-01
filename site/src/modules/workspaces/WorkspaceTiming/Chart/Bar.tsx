import { cn } from "#/utils/cn";
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

type BarProps = BaseBarProps<React.ComponentPropsWithRef<"div">>;

export const Bar: React.FC<BarProps> = ({
	colors,
	scale,
	value,
	offset,
	...htmlProps
}) => {
	return (
		<div
			{...htmlProps}
			className={cn(
				"relative flex h-[inherit] min-w-6 rounded-lg border border-border bg-surface-primary p-[7px]",
				"after:absolute after:inset-y-[-2px] after:left-[-8px] after:right-[-8px] after:content-['']",
				htmlProps.className,
			)}
			style={barStyle({ colors, scale, value, offset, style: htmlProps.style })}
		/>
	);
};

type ClickableBarProps = BaseBarProps<React.ComponentPropsWithRef<"button">>;

export const ClickableBar: React.FC<ClickableBarProps> = ({
	colors,
	scale,
	value,
	offset,
	...htmlProps
}) => {
	return (
		<button
			type="button"
			{...htmlProps}
			className={cn(
				"relative flex h-inherit min-w-[34px] cursor-pointer rounded-lg border border-border bg-surface-primary p-[7px]",
				"after:absolute after:inset-y-[-2px] after:left-[-8px] after:right-[-8px] after:content-['']",
				"outline-none hover:border-content-link focus-visible:border-content-link active:border-content-link",
				htmlProps.className,
			)}
			style={barStyle({ colors, scale, value, offset, style: htmlProps.style })}
		/>
	);
};

const barStyle = ({
	scale,
	value,
	colors,
	offset,
	style,
}: BaseBarProps<unknown> & {
	style?: React.CSSProperties;
}): React.CSSProperties => ({
	width: `calc((var(--x-axis-width) * ${value}) / ${scale})`,
	marginLeft: `calc((var(--x-axis-width) * ${offset}) / ${scale})`,
	backgroundColor: colors?.fill,
	borderColor: colors?.stroke,
	borderStyle: "solid",
	...style,
});
