import { type FC, type HTMLProps, useLayoutEffect, useRef } from "react";
import { cn } from "#/utils/cn";
import { formatTime } from "./utils";

const XAxisMinWidth = 130;

type XAxisProps = HTMLProps<HTMLDivElement> & {
	ticks: number[];
	scale: number;
};

export const XAxis: FC<XAxisProps> = ({ ticks, scale, ...htmlProps }) => {
	const rootRef = useRef<HTMLDivElement>(null);

	// The X axis should occupy all available space. If there is extra space,
	// increase the column width accordingly. Use a CSS variable to propagate the
	// value to the child components.
	useLayoutEffect(() => {
		const rootEl = rootRef.current;
		if (!rootEl) {
			return;
		}
		// We always add one extra column to the grid to ensure that the last column
		// is fully visible.
		const avgWidth = rootEl.clientWidth / (ticks.length + 1);
		const width = avgWidth > XAxisMinWidth ? avgWidth : XAxisMinWidth;
		rootEl.style.setProperty("--x-axis-width", `${width}px`);
	}, [ticks]);

	return (
		<div
			{...htmlProps}
			className={cn(
				"flex flex-col flex-1 min-h-full relative h-fit",
				"border-solid border-0 border-l",
				htmlProps.className,
			)}
			ref={rootRef}
		>
			<XAxisLabels>
				{ticks.map((tick) => (
					<XAxisLabel key={tick}>{formatTime(tick)}</XAxisLabel>
				))}
			</XAxisLabels>
			{htmlProps.children}
			<XGrid columns={ticks.length} />
		</div>
	);
};

const XAxisLabels: FC<HTMLProps<HTMLUListElement>> = (props) => {
	return (
		<ul
			{...props}
			className={cn(
				"sticky top-0 z-[2] bg-surface-primary",
				"flex items-center flex-shrink-0 list-none m-0",
				"w-fit p-0 min-w-full",
				"border-solid border-0 border-b",
				props.className,
			)}
			style={{
				minHeight: "var(--header-height)",
			}}
		/>
	);
};

const XAxisLabel: FC<HTMLProps<HTMLLIElement>> = (props) => {
	return (
		<li
			{...props}
			className={cn(
				"flex justify-center flex-shrink-0 text-content-secondary",
				// To centralize the labels between columns, we need to:
				// 1. Set the label width to twice the column width.
				// 2. Shift the label to the left by half of the column width.
				// Note: This adjustment is not applied to the first element,
				// as the 0 label/value is not displayed in the chart.
				"w-[calc(var(--x-axis-width)_*_2)]",
				"[&:not(:first-of-type)]:ml-[calc(-1*var(--x-axis-width))]",
				props.className,
			)}
		/>
	);
};

export const XAxisSection: FC<HTMLProps<HTMLDivElement>> = (props) => {
	return (
		<section
			{...props}
			className={cn(
				"flex flex-col",
				// Elevate this section to make it more prominent than the column dashes.
				"relative z-[1]",
				"[&:not(:first-of-type)]:pt-[calc(var(--section-padding)_+_var(--header-height))]",
				"[&:not(:first-of-type)]:border-solid",
				"[&:not(:first-of-type)]:border-0",
				"[&:not(:first-of-type)]:border-t",
				props.className,
			)}
			style={{
				...props.style,
				gap: "var(--x-axis-rows-gap)",
				padding: "var(--section-padding)",
			}}
		/>
	);
};

type XAxisRowProps = HTMLProps<HTMLDivElement> & {
	yAxisLabelId: string;
};

export const XAxisRow: FC<XAxisRowProps> = ({ yAxisLabelId, ...htmlProps }) => {
	const syncYAxisLabelHeightToXAxisRow = (rowEl: HTMLDivElement | null) => {
		if (!rowEl) {
			return;
		}
		// Selecting a label with special characters (e.g.,
		// #coder_metadata.container_info[0]) will fail because it is not a valid
		// selector. To handle this, we need to query by the id attribute and escape
		// it with quotes.
		const selector = `[id="${encodeURIComponent(yAxisLabelId)}"]`;
		const yAxisLabel = document.querySelector<HTMLSpanElement>(selector);
		if (!yAxisLabel) {
			console.warn(`Y-axis label with selector ${selector} not found.`);
			return;
		}
		yAxisLabel.style.height = `${rowEl.clientHeight}px`;
	};

	return (
		<section
			{...htmlProps}
			className={cn("flex items-center w-fit gap-2 h-8", htmlProps.className)}
			aria-labelledby={yAxisLabelId}
			ref={syncYAxisLabelHeightToXAxisRow}
		/>
	);
};

type XGridProps = HTMLProps<HTMLDivElement> & {
	columns: number;
};

const XGrid: FC<XGridProps> = ({ columns, ...htmlProps }) => {
	const borderDefault =
		typeof document === "undefined"
			? "hsl(var(--border-default))"
			: `hsl(${getComputedStyle(document.documentElement)
					.getPropertyValue("--border-default")
					.trim()})`;

	return (
		<div
			role="presentation"
			{...htmlProps}
			className={cn(
				"flex w-full h-full absolute top-0 left-0",
				htmlProps.className,
			)}
		>
			{[...Array(columns).keys()].map((key) => (
				<div
					key={key}
					className="flex-shrink-0 bg-repeat-y bg-right"
					style={{
						width: "var(--x-axis-width)",
						backgroundImage: `url("${dashedLine(borderDefault)}")`,
					}}
				/>
			))}
		</div>
	);
};

// A dashed line is used as a background image to create the grid.
// Using it as a background simplifies replication along the Y axis.
const dashedLine = (color: string) =>
	`data:image/svg+xml,${encodeURIComponent(`<svg width="2" height="446" viewBox="0 0 2 446" fill="none" xmlns="http://www.w3.org/2000/svg">
	<path fill-rule="evenodd" clip-rule="evenodd" d="M1.75 440.932L1.75 446L0.75 446L0.75 440.932L1.75 440.932ZM1.75 420.659L1.75 430.795L0.749999 430.795L0.749999 420.659L1.75 420.659ZM1.75 400.386L1.75 410.523L0.749998 410.523L0.749998 400.386L1.75 400.386ZM1.75 380.114L1.75 390.25L0.749998 390.25L0.749997 380.114L1.75 380.114ZM1.75 359.841L1.75 369.977L0.749997 369.977L0.749996 359.841L1.75 359.841ZM1.75 339.568L1.75 349.705L0.749996 349.705L0.749995 339.568L1.75 339.568ZM1.74999 319.295L1.74999 329.432L0.749995 329.432L0.749994 319.295L1.74999 319.295ZM1.74999 299.023L1.74999 309.159L0.749994 309.159L0.749994 299.023L1.74999 299.023ZM1.74999 278.75L1.74999 288.886L0.749993 288.886L0.749993 278.75L1.74999 278.75ZM1.74999 258.477L1.74999 268.614L0.749992 268.614L0.749992 258.477L1.74999 258.477ZM1.74999 238.204L1.74999 248.341L0.749991 248.341L0.749991 238.204L1.74999 238.204ZM1.74999 217.932L1.74999 228.068L0.74999 228.068L0.74999 217.932L1.74999 217.932ZM1.74999 197.659L1.74999 207.795L0.74999 207.795L0.749989 197.659L1.74999 197.659ZM1.74999 177.386L1.74999 187.523L0.749989 187.523L0.749988 177.386L1.74999 177.386ZM1.74999 157.114L1.74999 167.25L0.749988 167.25L0.749987 157.114L1.74999 157.114ZM1.74999 136.841L1.74999 146.977L0.749987 146.977L0.749986 136.841L1.74999 136.841ZM1.74999 116.568L1.74999 126.705L0.749986 126.705L0.749986 116.568L1.74999 116.568ZM1.74998 96.2955L1.74999 106.432L0.749985 106.432L0.749985 96.2955L1.74998 96.2955ZM1.74998 76.0228L1.74998 86.1591L0.749984 86.1591L0.749984 76.0228L1.74998 76.0228ZM1.74998 55.7501L1.74998 65.8864L0.749983 65.8864L0.749983 55.7501L1.74998 55.7501ZM1.74998 35.4774L1.74998 45.6137L0.749982 45.6137L0.749982 35.4774L1.74998 35.4774ZM1.74998 15.2047L1.74998 25.341L0.749982 25.341L0.749981 15.2047L1.74998 15.2047ZM1.74998 -4.37114e-08L1.74998 5.0683L0.749981 5.0683L0.749981 0L1.74998 -4.37114e-08Z" fill="${color}"/>
</svg>`)}`;
