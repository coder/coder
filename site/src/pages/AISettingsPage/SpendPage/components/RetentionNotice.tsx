import dayjs from "dayjs";
import type { FC } from "react";
import { Alert } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

interface RetentionNoticeProps {
	requestedStart: Date;
	appliedStart: string;
}

// The server raises start_date to the AI Gateway retention boundary because
// older records are purged, so a window that reaches further back covers less
// than the date picker shows.
export const RetentionNotice: FC<RetentionNoticeProps> = ({
	requestedStart,
	appliedStart,
}) => {
	const applied = new Date(appliedStart);
	if (applied.getTime() <= requestedStart.getTime()) {
		return null;
	}
	return (
		<Alert severity="info">
			Showing spend since {dayjs(applied).format("MMM D, YYYY h:mm A")}. Older
			AI Gateway records have been removed by the{" "}
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
