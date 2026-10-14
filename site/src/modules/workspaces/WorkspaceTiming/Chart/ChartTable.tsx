import { cn } from "cn";
import { useLayoutEffect, useRef } from "react";
import { yAxisWidth } from "./Chart";
import { XAxisTicks, XGrid } from "./XAxis";

const XAxisMinWidth = 130;

type ChartTableProps = {
	/** Accessible name of the table. It is visually hidden. */
	caption: string;
	ticks: number[];
	children: React.ReactNode;
};

/**
 * Renders the chart as a data table. The first column holds the row labels
 * and the second column holds the timeline, so each label shares a row with
 * its bar and duration.
 */
export const ChartTable: React.FC<ChartTableProps> = ({
	caption,
	ticks,
	children,
}) => {
	const rootRef = useRef<HTMLDivElement>(null);

	// The timeline should occupy all available space. If there is extra space,
	// increase the column width accordingly. Use a CSS variable to propagate the
	// value to the child components.
	useLayoutEffect(() => {
		const rootEl = rootRef.current;
		if (!rootEl) {
			return;
		}
		// We always add one extra column to the grid to ensure that the last column
		// is fully visible. The 1px is the border between the two columns.
		const timelineWidth = rootEl.clientWidth - yAxisWidth - 1;
		const avgWidth = timelineWidth / (ticks.length + 1);
		const width = Math.max(avgWidth, XAxisMinWidth);
		rootEl.style.setProperty("--x-axis-width", `${width}px`);
	}, [ticks]);

	return (
		<div ref={rootRef} className="relative h-fit min-h-full flex-1">
			<XGrid columns={ticks.length} />
			<table className="relative z-1 w-full border-separate border-spacing-0">
				<caption className="sr-only">{caption}</caption>
				{children}
			</table>
		</div>
	);
};

type ChartTableSectionProps = {
	/** Section name, rendered as the header of this group of rows. */
	label: React.ReactNode;
	/**
	 * Pass to the first section only. Its header row renders the tick labels
	 * and the header of the duration column.
	 */
	ticks?: number[];
	children: React.ReactNode;
};

/** A group of rows under a section header, rendered as a `<tbody>`. */
export const ChartTableSection: React.FC<ChartTableSectionProps> = ({
	label,
	ticks,
	children,
}) => {
	return (
		<tbody className="[&>tr:nth-child(2)>*]:pt-4 [&>tr:not(:first-child):last-child>*]:pb-4">
			<tr>
				<th
					scope="rowgroup"
					className={cn(
						"sticky top-0 z-2 bg-surface-primary p-0 border-b",
						"px-(--section-padding) text-left text-xs font-medium text-content-secondary",
						// Cell heights include borders, so later sections add 1px for the
						// top border that separates them from the previous section.
						ticks
							? "h-(--header-height)"
							: "h-[calc(var(--header-height)+1px)] border-t",
					)}
				>
					{label}
				</th>
				{ticks ? (
					<th
						scope="col"
						className="sticky top-0 z-2 h-(--header-height) bg-surface-primary p-0 border-b border-l font-medium"
					>
						<span className="sr-only">Duration</span>
						<XAxisTicks ticks={ticks} />
					</th>
				) : (
					<td className="border-t border-l" />
				)}
			</tr>
			{children}
		</tbody>
	);
};

type ChartTableRowProps = {
	/** Row header content, such as the stage or resource name. */
	label: React.ReactNode;
	/** Timeline content, usually a bar followed by the duration. */
	children: React.ReactNode;
};

/** A row with a header cell for the label and a cell for the timeline. */
export const ChartTableRow: React.FC<ChartTableRowProps> = ({
	label,
	children,
}) => {
	return (
		<tr>
			<th
				scope="row"
				className="px-(--section-padding) py-2.5 text-right font-medium"
			>
				<span className="block w-[calc(var(--y-axis-width)-2*var(--section-padding))] overflow-hidden text-ellipsis whitespace-nowrap">
					{label}
				</span>
			</th>
			<td className="px-(--section-padding) py-2.5 border-l">
				<div className="flex h-8 w-fit items-center gap-2">{children}</div>
			</td>
		</tr>
	);
};
