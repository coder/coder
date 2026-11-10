import { cn } from "cn";

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
};

const barClassName = cn(
	"relative flex h-[inherit] min-w-6 rounded-lg border border-border bg-surface-primary p-1.75",
	"after:absolute after:inset-y-[-2px] after:left-[-8px] after:right-[-8px] after:content-['']",
);

type BarProps = BaseBarProps<React.ComponentProps<"div">>;

export const Bar: React.FC<BarProps> = ({
	scale,
	value,
	offset,
	className,
	style,
	...htmlProps
}) => {
	return (
		<div
			{...htmlProps}
			className={cn(barClassName, className)}
			style={barStyle({ scale, value, offset, style })}
		/>
	);
};

type ClickableBarProps = BaseBarProps<React.ComponentProps<"button">>;

export const ClickableBar: React.FC<ClickableBarProps> = ({
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
				"relative flex h-inherit min-w-8.5 cursor-pointer rounded-lg border border-border bg-surface-primary p-1.75",
				"after:absolute after:-inset-y-0.5 after:-left-2 after:-right-2 after:content-['']",
				"outline-hidden hover:border-content-link focus-visible:border-content-link active:border-content-link",
				htmlProps.className,
			)}
			style={barStyle({ scale, value, offset, style: htmlProps.style })}
		/>
	);
};

type FocusableBarProps = BaseBarProps<React.ComponentProps<"button">> & {
	"aria-label": string;
};

/**
 * A bar without an action of its own. It is focusable so keyboard users can
 * open its tooltip.
 */
export const FocusableBar: React.FC<FocusableBarProps> = ({
	scale,
	value,
	offset,
	className,
	style,
	...htmlProps
}) => {
	return (
		<button
			type="button"
			{...htmlProps}
			className={cn(
				barClassName,
				"focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
				className,
			)}
			style={barStyle({ scale, value, offset, style })}
		/>
	);
};

const barStyle = ({
	scale,
	value,
	offset,
	style,
}: BaseBarProps<unknown> & {
	style?: React.CSSProperties;
}): React.CSSProperties => ({
	width: `calc((var(--x-axis-width) * ${value}) / ${scale})`,
	marginLeft: `calc((var(--x-axis-width) * ${offset}) / ${scale})`,
	borderStyle: "solid",
	...style,
});
