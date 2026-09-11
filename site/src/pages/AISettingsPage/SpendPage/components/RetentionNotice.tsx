import dayjs from "dayjs";
import type { FC } from "react";
import { Alert } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

interface RetentionNoticeProps {
	requestedStart: Date;
	applied: { start_date: string; end_date: string };
}

// The server raises start_date to the AI Gateway retention boundary because
// older records are purged, so a window that reaches further back covers less
// than the date picker shows. A window that ends before the boundary comes
// back empty, with start_date equal to end_date.
export const RetentionNotice: FC<RetentionNoticeProps> = ({
	requestedStart,
	applied,
}) => {
	const appliedStart = new Date(applied.start_date);
	if (appliedStart.getTime() <= requestedStart.getTime()) {
		return null;
	}
	const isEmpty =
		appliedStart.getTime() >= new Date(applied.end_date).getTime();
	return (
		<Alert severity="info">
			{isEmpty
				? "No AI Gateway spend is retained for the selected period. "
				: `Showing spend since ${dayjs(appliedStart).format("MMM D, YYYY h:mm A")}. `}
			Older AI Gateway records have been removed by the{" "}
			<Link
				href={docs("/ai-coder/ai-gateway/monitoring#data-retention")}
				target="_blank"
				rel="noreferrer"
			>
				data retention period
			</Link>
			.
		</Alert>
	);
};
