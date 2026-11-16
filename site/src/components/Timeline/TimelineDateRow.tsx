import { TableCell, TableRow } from "#/components/Table/Table";
import { formatDate } from "#/utils/time";
import { createDisplayDate } from "./utils";

type TimelineDateRowProps = {
	date: Date;
};

export const TimelineDateRow: React.FC<TimelineDateRowProps> = ({ date }) => {
	return (
		<TableRow>
			<TableCell
				className="py-2 px-8 bg-surface-secondary text-xs relative text-content-secondary capitalize"
				title={formatDate(date)}
			>
				<h3 className="m-0 text-xs">{createDisplayDate(date)}</h3>
			</TableCell>
		</TableRow>
	);
};
