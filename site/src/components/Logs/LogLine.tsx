import type { FC, HTMLAttributes } from "react";
import type { LogLevel } from "#/api/typesGenerated";
import { cn } from "#/utils/cn";

const DEFAULT_LOG_LINE_SIDE_PADDING = 24;

export interface Line {
	id: number;
	time: string;
	output: string;
	level: LogLevel;
	sourceId: string;
}

type LogLineProps = {
	level: LogLevel;
} & HTMLAttributes<HTMLPreElement>;

export const LogLine: FC<LogLineProps> = ({
	level,
	className,
	style,
	...props
}) => {
	return (
		<pre
			{...props}
			className={cn(
				"logs-line",
				"m-0 break-all flex items-center h-auto",
				"text-[13px] text-content-primary font-mono",
				level === "error" &&
					"bg-surface-error text-content-error [&_.dashed-line]:bg-border-error",
				level === "debug" &&
					"bg-surface-sky text-content-sky [&_.dashed-line]:bg-border-sky",
				level === "warn" &&
					"bg-surface-warning text-content-warning [&_.dashed-line]:bg-border-warning",
				className,
			)}
			style={{
				...style,
				padding:
					style?.padding ??
					`0 var(--log-line-side-padding, ${DEFAULT_LOG_LINE_SIDE_PADDING}px)`,
			}}
		/>
	);
};

export const LogLinePrefix: FC<HTMLAttributes<HTMLSpanElement>> = ({
	className,
	...props
}) => {
	return (
		<pre
			className={cn(
				"select-none m-0 inline-block text-content-secondary mr-6",
				className,
			)}
			{...props}
		/>
	);
};
