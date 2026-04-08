import AnsiToHTML from "ansi-to-html";
import dayjs from "dayjs";
import { type FC, type ReactNode, useMemo } from "react";
import { type Line, LogLine, LogLinePrefix } from "#/components/Logs/LogLine";
// Approximate height of a log line. Used to control virtualized list height.
export const AGENT_LOG_LINE_HEIGHT = 20;

const convert = new AnsiToHTML();

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
	const output = useMemo(() => {
		return convert.toHtml(line.output.split(/\r/g).pop() as string);
	}, [line.output]);
	const timestamp = useMemo(() => {
		return dayjs(line.time).format("HH:mm:ss.SSS");
	}, [line.time]);

	return (
		<LogLine className="pl-4" level={line.level} style={style}>
			{sourceIcon}
			<LogLinePrefix>{timestamp}</LogLinePrefix>
			<span
				// biome-ignore lint/security/noDangerouslySetInnerHtml: Output contains HTML to represent ANSI-code formatting
				dangerouslySetInnerHTML={{
					__html: output,
				}}
			/>
		</LogLine>
	);
};
