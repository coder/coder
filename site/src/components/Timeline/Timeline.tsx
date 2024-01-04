import { TimelineDateRow } from "components/Timeline/TimelineDateRow";
import { FC, Fragment } from "react";
import { createDisplayDate } from "./utils";

type GetDateFn<TData> = (data: TData) => Date;

const groupByDate = <TData,>(
  items: TData[],
  getDate: GetDateFn<TData>,
): Record<string, TData[]> => {
  const itemsByDate: Record<string, TData[]> = {};

  items.forEach((item) => {
    const dateKey = getDate(item).toDateString();

    if (dateKey in itemsByDate) {
      itemsByDate[dateKey].push(item);
    } else {
      itemsByDate[dateKey] = [item];
    }
  });

  return itemsByDate;
};

export interface TimelineProps<TData> {
  items: TData[];
  getDate: GetDateFn<TData>;
  row: (item: TData) => JSX.Element;
  dateRow?: FC<{ date: Date; displayDate: string }>;
}

export const Timeline = <TData,>({
  items,
  getDate,
  row,
  dateRow: DateRow = TimelineDateRow,
}: TimelineProps<TData>): JSX.Element => {
  const itemsByDate = groupByDate(items, getDate);

  return (
    <>
      {Object.keys(itemsByDate).map((dateStr) => {
        const items = itemsByDate[dateStr];
        const date = new Date(dateStr);
        const displayDate = createDisplayDate(date);

        return (
          <Fragment key={dateStr}>
            <DateRow date={date} displayDate={displayDate} />
            {items.map(row)}
          </Fragment>
        );
      })}
    </>
  );
};
