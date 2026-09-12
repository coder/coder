import type { FC } from "react";
import { TableCell, TableRow } from "#/components/Table/Table";
import { formatDate } from "#/utils/time";
import { createDisplayDate } from "./utils";

interface TimelineDateRowProps {
	date: Date;
}

export const TimelineDateRow: FC<TimelineDateRowProps> = ({ date }) => {
	return (
		<TableRow>
			<TableCell
				className="py-2 px-8 bg-surface-secondary text-xs relative text-content-secondary capitalize"
				title={formatDate(date)}
			>
				{createDisplayDate(date)}
			</TableCell>
		</TableRow>
	);
};
