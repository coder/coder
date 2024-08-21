import type { Interpolation, Theme } from "@emotion/react";
import type { LogLevel } from "api/typesGenerated";
import type { FC, HTMLAttributes } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";

export const DEFAULT_LOG_LINE_SIDE_PADDING = 24;

export interface Line {
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
			className={`${level} ${divProps.className} logs-line`}
			{...divProps}
		/>
	);
};

export const LogLinePrefix: FC<HTMLAttributes<HTMLSpanElement>> = (props) => {
	return <pre css={styles.prefix} {...props} />;
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
			backgroundColor: theme.colorRoles.error.background,
			color: theme.colorRoles.error.text,

			"& .dashed-line": {
				backgroundColor: theme.colorRoles.error.outline,
			},
		},

		"&.debug": {
			backgroundColor: theme.colorRoles.notice.background,
			color: theme.colorRoles.notice.text,

			"& .dashed-line": {
				backgroundColor: theme.colorRoles.notice.outline,
			},
		},

		"&.warn": {
			backgroundColor: theme.colorRoles.warning.background,
			color: theme.colorRoles.warning.text,

			"& .dashed-line": {
				backgroundColor: theme.colorRoles.warning.outline,
			},
		},
	}),

	prefix: (theme) => ({
		userSelect: "none",
		margin: 0,
		display: "inline-block",
		color: theme.palette.text.secondary,
		marginRight: 24,
	}),
} satisfies Record<string, Interpolation<Theme>>;
