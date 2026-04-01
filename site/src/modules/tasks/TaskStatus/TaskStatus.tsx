import type * as TypesGen from "api/typesGenerated";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "components/StatusIndicator/StatusIndicator";
import type { FC } from "react";

type TaskStatusProps = {
	status: TypesGen.TaskStatus;
	stateMessage: string;
};

export const taskStatusToStatusIndicatorVariant: Record<
	TypesGen.TaskStatus,
	StatusIndicatorProps["variant"]
> = {
	active: "success",
	error: "failed",
	initializing: "pending",
	pending: "pending",
	paused: "inactive",
	unknown: "warning",
};

export const TaskStatus: FC<TaskStatusProps> = ({ status, stateMessage }) => {
	return (
		<StatusIndicator
			variant={taskStatusToStatusIndicatorVariant[status]}
			className="items-start"
		>
			<StatusIndicatorDot className="mt-1" />
			<div className="flex flex-col">
				<span className="[&:first-letter]:uppercase">{status}</span>
				<span className="text-xs font-normal text-content-secondary truncate max-w-sm">
					{stateMessage}
				</span>
			</div>
		</StatusIndicator>
	);
};
