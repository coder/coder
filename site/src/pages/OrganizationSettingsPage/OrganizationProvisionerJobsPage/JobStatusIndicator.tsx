import type { ProvisionerJob, ProvisionerJobStatus } from "api/typesGenerated";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "components/StatusIndicator/StatusIndicator";
import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";

const variantByStatus: Record<
	ProvisionerJobStatus,
	StatusIndicatorProps["variant"]
> = {
	succeeded: "success",
	failed: "failed",
	pending: "pending",
	running: "pending",
	canceling: "pending",
	canceled: "inactive",
	unknown: "inactive",
};

type JobStatusIndicatorProps = {
	job: ProvisionerJob;
};

export const JobStatusIndicator: FC<JobStatusIndicatorProps> = ({ job }) => {
	return (
		<StatusIndicator size="sm" variant={variantByStatus[job.status]}>
			<StatusIndicatorDot />
			<span className="[&:first-letter]:uppercase">{job.status}</span>
			{job.status === "failed" && (
				<TriangleAlertIcon className="size-icon-xs p-[1px]" />
			)}
			{job.status === "pending" && `(${job.queue_position}/${job.queue_size})`}
		</StatusIndicator>
	);
};
