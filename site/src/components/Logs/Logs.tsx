import type { Interpolation, Theme } from "@emotion/react";
import dayjs from "dayjs";
import type { FC } from "react";
import { cn } from "utils/cn";
import { type Line, LogLine, LogLinePrefix } from "./LogLine";

export const DEFAULT_LOG_LINE_SIDE_PADDING = 24;

interface LogsProps {
	lines: Line[];
	hideTimestamps?: boolean;
	className?: string;
}

export const Logs: FC<LogsProps> = ({
	hideTimestamps,
	lines,
	className = "",
}) => {
	return (
		<div
			className={cn(
				"logs-container",
				"min-h-[156px] py-2 rounded-lg overflow-x-auto bg-surface-primary",
				"[&:not(:last-child)]:border-0 [&:not(:last-child)]:border-b",
				"[&:not(:last-child)]:rounded-none [&:not(:last-child)]:border-b-border",
				className,
			)}
		>
			<div className="min-w-fit">
				{lines.map((line) => (
					<LogLine key={line.id} level={line.level}>
						{!hideTimestamps && (
							<LogLinePrefix>
								{dayjs(line.time).format("HH:mm:ss.SSS")}
							</LogLinePrefix>
						)}
						<span>{line.output}</span>
					</LogLine>
				))}
			</div>
		</div>
	);
};
