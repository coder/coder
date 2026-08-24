import dayjs from "dayjs";
import { AnsiHtml } from "fancy-ansi/react";
import { type FC, type ReactNode, useMemo } from "react";
import { type Line, LogLine, LogLinePrefix } from "#/components/Logs/LogLine";
// Approximate height of a log line. Used to control virtualized list height.
export const AGENT_LOG_LINE_HEIGHT = 20;

interface AgentLogLineProps {
	line: Line;
	style: React.CSSProperties;
	sourceIcon: ReactNode;
}

export const AgentLogLine: FC<AgentLogLineProps> = ({
	line,
	sourceIcon,
	style,
}) => {
	// Only render the text after the last carriage return so progress-bar style
	// output that redraws a single line shows its final state.
	const lastCarriageReturn = line.output.lastIndexOf("\r");
	const output =
		lastCarriageReturn === -1
			? line.output
			: line.output.slice(lastCarriageReturn + 1);
	const timestamp = useMemo(() => {
		return dayjs(line.time).format("HH:mm:ss.SSS");
	}, [line.time]);

	return (
		<LogLine className="pl-4" level={line.level} style={style}>
			{sourceIcon}
			<LogLinePrefix>{timestamp}</LogLinePrefix>
			<AnsiHtml text={output} />
		</LogLine>
	);
};
