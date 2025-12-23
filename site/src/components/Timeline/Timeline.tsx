import { TimelineDateRow } from "components/Timeline/TimelineDateRow";
import { Fragment, type JSX } from "react";

type GetDateFn<TData> = (data: TData) => Date;

const groupByDate = <TData,>(
	items: readonly TData[],
	getDate: GetDateFn<TData>,
): Record<string, TData[]> => {
	const itemsByDate: Record<string, TData[]> = {};

	for (const item of items) {
		const dateKey = getDate(item).toDateString();

		if (dateKey in itemsByDate) {
			itemsByDate[dateKey]!.push(item);
		} else {
			itemsByDate[dateKey]! = [item];
		}
	}

	return itemsByDate;
};

interface TimelineProps<TData> {
	items: readonly TData[];
	getDate: GetDateFn<TData>;
	row: (item: TData) => JSX.Element;
}

export const Timeline = <TData,>({
	items,
	getDate,
	row,
}: TimelineProps<TData>): JSX.Element => {
	const itemsByDate = groupByDate(items, getDate);

	return (
		<>
			{Object.entries(itemsByDate).map(([dateStr, items]) => (
				<Fragment key={dateStr}>
					<TimelineDateRow date={new Date(dateStr)} />
					{items.map(row)}
				</Fragment>
			))}
		</>
	);
};
