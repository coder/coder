import type { ProvisionerJobStatus } from "api/typesGenerated";
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
	status: ProvisionerJobStatus;
	queue?: { size: number; position: number };
};

export const JobStatusIndicator: FC<JobStatusIndicatorProps> = ({
	status,
	queue,
}) => {
	return (
		<StatusIndicator size="sm" variant={variantByStatus[status]}>
			<StatusIndicatorDot />
			<span className="[&:first-letter]:uppercase">{status}</span>
			{status === "failed" && (
				<TriangleAlertIcon className="size-icon-xs p-[1px]" />
			)}
			{status === "pending" && queue && `(${queue.position}/${queue.size})`}
		</StatusIndicator>
	);
};
