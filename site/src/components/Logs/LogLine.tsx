import type { Interpolation, Theme } from "@emotion/react";
import type { LogLevel } from "api/typesGenerated";
import type { FC, HTMLAttributes } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { cn } from "utils/cn";

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

export const LogLine: FC<LogLineProps> = ({ level, ...divProps }) => {
	return (
		<pre
			css={styles.line}
			{...divProps}
			className={cn(
				"m-0 break-all flex items-center text-[13px]",
				"text-content-primary font-mono h-auto",
				level,
				divProps.className,
				"logs-line",
			)}
		/>
	);
};

export const LogLinePrefix: FC<HTMLAttributes<HTMLSpanElement>> = (props) => {
	return (
		<pre
			{...props}
			className={cn([
				"select-none m-0 inline-block text-content-secondary mr-6 text-right flex-shrink-0",
				props.className,
			])}
		/>
	);
};

const styles = {
	line: (theme) => ({
		margin: 0,
		wordBreak: "break-all",
		display: "flex",
		alignItems: "center",
		fontSize: 13,
		color: theme.palette.text.primary,
		fontFamily: MONOSPACE_FONT_FAMILY,
		height: "auto",
		padding: `0 var(--log-line-side-padding, ${DEFAULT_LOG_LINE_SIDE_PADDING}px)`,

		"&.error": {
			backgroundColor: theme.roles.error.background,
			color: theme.roles.error.text,

			"& .dashed-line": {
				backgroundColor: theme.roles.error.outline,
			},
		},

		"&.debug": {
			backgroundColor: theme.roles.notice.background,
			color: theme.roles.notice.text,

			"& .dashed-line": {
				backgroundColor: theme.roles.notice.outline,
			},
		},

		"&.warn": {
			backgroundColor: theme.roles.warning.background,
			color: theme.roles.warning.text,

			"& .dashed-line": {
				backgroundColor: theme.roles.warning.outline,
			},
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
