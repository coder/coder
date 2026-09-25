import dayjs from "dayjs";
import { AnsiHtml } from "fancy-ansi/react";
import { type FC, type ReactNode, useMemo } from "react";
import { type Line, LogLine, LogLinePrefix } from "#/components/Logs/LogLine";
// Approximate height of a log line. Used to control virtualized list height.
export const AGENT_LOG_LINE_HEIGHT = 20;

type AgentLogLineProps = {
	line: Line;
	style?: React.CSSProperties;
	sourceIcon: ReactNode;
};

/**
 * Agent log output with ANSI colors. Shows only the text after the last
 * carriage return, so a redrawn progress line shows its final state.
 */
export const AgentLogOutput: FC<{ output: string }> = ({ output }) => {
	const lastCarriageReturn = output.lastIndexOf("\r");
	return (
		<AnsiHtml
			text={
				lastCarriageReturn === -1
					? output
					: output.slice(lastCarriageReturn + 1)
			}
		/>
	);
};

export const AgentLogLine: FC<AgentLogLineProps> = ({
	line,
	sourceIcon,
	style,
}) => {
	const timestamp = useMemo(() => {
		return dayjs(line.time).format("HH:mm:ss.SSS");
	}, [line.time]);

	return (
		<LogLine className="pl-4 min-h-5" level={line.level} style={style}>
			{sourceIcon}
			<LogLinePrefix>{timestamp}</LogLinePrefix>
			<AgentLogOutput output={line.output} />
		</LogLine>
	);
};
