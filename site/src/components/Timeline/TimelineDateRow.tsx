import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import type { FC } from "react";
import { createDisplayDate } from "./utils";

export interface TimelineDateRow {
	date: Date;
}

export const TimelineDateRow: FC<TimelineDateRow> = ({ date }) => {
	return (
		<TableRow className="[&:not:first-of-type_td]:border-t [&:not:first-of-type_td]:border-solid">
			<TableCell
				className="px-2! py-8! text-xs relative text-content-secondary bg-surface-secondary capitalize"
				title={date.toLocaleDateString()}
			>
				{createDisplayDate(date)}
			</TableCell>
		</TableRow>
	);
};
