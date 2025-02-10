import type {
	ProvisionerDaemonJob,
	ProvisionerJob,
	ProvisionerJobStatus,
} from "api/typesGenerated";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "components/StatusIndicator/StatusIndicator";
import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";

type JobStatusIndicatorProps = {
	job: ProvisionerJob | ProvisionerDaemonJob;
};

export const JobStatusIndicator: FC<JobStatusIndicatorProps> = ({ job }) => {
	const isProvisionerJob = "queue_position" in job;
	return (
		<StatusIndicator size="sm" variant={statusIndicatorVariant(job.status)}>
			<StatusIndicatorDot />
			<span className="[&:first-letter]:uppercase">{job.status}</span>
			{job.status === "failed" && (
				<TriangleAlertIcon className="size-icon-xs p-[1px]" />
			)}
			{job.status === "pending" &&
				isProvisionerJob &&
				`(${job.queue_position}/${job.queue_size})`}
		</StatusIndicator>
	);
};

function statusIndicatorVariant(
	status: ProvisionerJobStatus,
): StatusIndicatorProps["variant"] {
	switch (status) {
		case "succeeded":
			return "success";
		case "failed":
			return "failed";
		case "pending":
		case "running":
		case "canceling":
			return "pending";
		case "canceled":
		case "unknown":
			return "inactive";
	}
}
