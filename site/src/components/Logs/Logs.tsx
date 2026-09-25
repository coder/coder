import { cn } from "cn";
import dayjs from "dayjs";
import type { FC, ReactNode } from "react";
import { type Line, LogLine, LogLinePrefix } from "./LogLine";

const DEFAULT_LOG_LINE_SIDE_PADDING = 24;

type LogsHeaderProps = {
	title: ReactNode;
	/** Right-aligned secondary text. */
	detail?: ReactNode;
};

/** Section header above a `Logs` list, such as a build stage. */
export const LogsHeader: FC<LogsHeaderProps> = ({ title, detail }) => {
	return (
		<div
			className={cn(
				"logs-header",
				"flex items-center border-solid border-0 border-b last:border-b-0 border-border font-sans",
				"bg-surface-primary text-xs font-semibold leading-none",
				"first-of-type:pt-4",
			)}
			style={{
				padding: `12px var(--log-line-side-padding, ${DEFAULT_LOG_LINE_SIDE_PADDING}px)`,
			}}
		>
			<div>{title}</div>
			{detail && (
				<div className="ml-auto text-xs text-content-secondary">{detail}</div>
			)}
		</div>
	);
};

type LogsProps = {
	lines: Line[];
	hideTimestamps?: boolean;
	className?: string;
	/** Renders each line's output. Defaults to plain text. */
	LineOutput?: FC<{ output: string }>;
};

export const Logs: FC<LogsProps> = ({
	hideTimestamps,
	lines,
	className = "",
	LineOutput,
}) => {
	return (
		<div
			className={cn(
				"logs-container",
				"min-h-40 py-2 rounded-lg overflow-x-auto bg-surface-primary",
				"not-last:border-0",
				"not-last:border-solid",
				"not-last:border-b-border",
				"not-last:rounded-none",
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
						{LineOutput ? (
							<LineOutput output={line.output} />
						) : (
							<span>{line.output}</span>
						)}
					</LogLine>
				))}
			</div>
		</div>
	);
};
