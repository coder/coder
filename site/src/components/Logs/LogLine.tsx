import { cn } from "cn";
import type { LogLevel } from "#/api/typesGenerated";

const DEFAULT_LOG_LINE_SIDE_PADDING = 24;

export type Line = {
	id: number;
	time: string;
	output: string;
	level: LogLevel;
	sourceId: string;
};

type LogLineProps = {
	level: LogLevel;
} & React.ComponentProps<"pre">;

export const LogLine: React.FC<LogLineProps> = ({
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
				"text-xs font-normal text-content-primary font-mono",
				level === "error" &&
					"bg-surface-red text-content-destructive [&_.dashed-line]:bg-border-destructive",
				level === "debug" &&
					"bg-surface-sky text-highlight-sky [&_.dashed-line]:bg-border-pending",
				level === "warn" &&
					"bg-surface-orange text-content-warning [&_.dashed-line]:bg-border-warning",
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

export const LogLinePrefix: React.FC<React.ComponentProps<"pre">> = ({
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
