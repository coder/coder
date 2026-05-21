import type { FC } from "react";
import { TableCell, TableRow } from "#/components/Table/Table";
import { formatDate } from "#/utils/time";
import { createDisplayDate } from "./utils";

export interface TimelineDateRow {
	date: Date;
}

export const TimelineDateRow: FC<TimelineDateRow> = ({ date }) => {
	return (
		<TableRow className="[&:not(:first-of-type)_td]:border-t [&:not(:first-of-type)_td]:border-border">
			<TableCell
				className="!py-2 !px-8 !bg-surface-primary text-xs relative text-content-secondary capitalize"
				title={formatDate(date)}
			>
				{createDisplayDate(date)}
			</TableCell>
		</TableRow>
	);
};
