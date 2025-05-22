import type { Interpolation, Theme } from "@emotion/react";
import { format, parseISO } from "date-fns";
import type { FC } from "react";
import { type Line, LogLine, LogLinePrefix } from "./LogLine";

export const DEFAULT_LOG_LINE_SIDE_PADDING = 24;

export interface LogsProps {
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
		<div css={styles.root} className={`${className} logs-container`}>
			<div css={{ minWidth: "fit-content" }}>
				{lines.map((line) => (
					<LogLine key={line.id} level={line.level}>
						{!hideTimestamps && (
							<LogLinePrefix>
								{format(
									typeof line.time === "string"
										? parseISO(line.time)
										: new Date(line.time),
									"HH:mm:ss.SSS",
								)}
							</LogLinePrefix>
						)}
						<span>{line.output}</span>
					</LogLine>
				))}
			</div>
		</div>
	);
};

const styles = {
	root: (theme) => ({
		minHeight: 156,
		padding: "8px 0",
		borderRadius: 8,
		overflowX: "auto",
		background: theme.palette.background.default,

		"&:not(:last-child)": {
			borderBottom: `1px solid ${theme.palette.divider}`,
			borderRadius: 0,
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
