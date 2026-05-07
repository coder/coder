import type { FC } from "react";
import { cn } from "#/utils/cn";
import { roundDurationDisplay } from "../../utils";

interface AgenticLoopTableProps {
	duration: number; // in seconds
	toolCalls: number;
	className?: string;
}

export const AgenticLoopTable: FC<AgenticLoopTableProps> = ({
	duration,
	toolCalls,
	className,
}) => {
	return (
		<div
			className={cn(
				"text-sm text-content-secondary font-normal flex flex-col gap-1",
				className,
			)}
		>
			<div className="flex items-center justify-between h-6">
				<span className="pr-4">Tool calls</span>
				<span>{toolCalls}</span>
			</div>
			<div className="flex items-center justify-between h-6">
				<span className="pr-4">Duration</span>
				<span title={`${duration}ms`}>{roundDurationDisplay(duration)}</span>
			</div>
		</div>
	);
};
