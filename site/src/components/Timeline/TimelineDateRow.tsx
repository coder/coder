import { TableCell, TableRow } from "components/Table/Table";
import type { FC } from "react";
import { cn } from "utils/cn";
import { createDisplayDate } from "./utils";

export interface TimelineDateRow {
	date: Date;
}

export const TimelineDateRow: FC<TimelineDateRow> = ({ date }) => {
	return (
		<TableRow
			className={cn([
				"[&:not(:first-of-type)_td]:border-0",
				"[&:not(:first-of-type)_td]:border-t",
				"[&:not(:first-of-type)_td]:border-border",
				"[&:not(:first-of-type)_td]:border-solid",
			])}
		>
			<TableCell
				className="!py-2 !px-8 bg-surface-primary text-xs relative text-content-secondary capitalize"
				title={date.toLocaleDateString()}
			>
				{createDisplayDate(date)}
			</TableCell>
		</TableRow>
	);
};
