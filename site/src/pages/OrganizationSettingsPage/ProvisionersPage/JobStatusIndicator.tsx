import type {
	ProvisionerDaemonJob,
	ProvisionerJob,
	ProvisionerJobStatus,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "components/StatusIndicator/StatusIndicator";
import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";

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

type DaemonJobStatusIndicatorProps = {
	job: ProvisionerDaemonJob;
};

export const DaemonJobStatusIndicator: FC<DaemonJobStatusIndicatorProps> = ({
	job,
}) => {
	return (
		<StatusIndicator size="sm" variant={variantByStatus[job.status]}>
			<StatusIndicatorDot />
			<span className="[&:first-letter]:uppercase">{job.status}</span>
			{job.status === "failed" && (
				<TriangleAlertIcon className="size-icon-xs p-[1px]" />
			)}
			<Button size="xs" variant="outline" asChild>
				<RouterLink to={`?id=${job.id}`}>View job</RouterLink>
			</Button>
		</StatusIndicator>
	);
};
