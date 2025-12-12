import AnsiToHTML from "ansi-to-html";
import { type Line, LogLine, LogLinePrefix } from "components/Logs/LogLine";
import { type FC, type ReactNode, useMemo } from "react";
// Approximate height of a log line. Used to control virtualized list height.
export const AGENT_LOG_LINE_HEIGHT = 20;

const convert = new AnsiToHTML();

interface AgentLogLineProps {
	line: Line;
	number: number;
	style: React.CSSProperties;
	sourceIcon: ReactNode;
	maxLineNumber: number;
}

export const AgentLogLine: FC<AgentLogLineProps> = ({
	line,
	number,
	maxLineNumber,
	sourceIcon,
	style,
}) => {
	const output = useMemo(() => {
		return convert.toHtml(line.output.split(/\r/g).pop() as string);
	}, [line.output]);

	return (
		<LogLine className="pl-4" level={line.level} style={style}>
			{sourceIcon}
			<LogLinePrefix
				style={{
					minWidth: `${maxLineNumber.toString().length - 1}em`,
				}}
				className="w-[32px] text-right text-content-disabled flex-shrink-0"
			>
				{number}
			</LogLinePrefix>
			<span
				// biome-ignore lint/security/noDangerouslySetInnerHtml: Output contains HTML to represent ANSI-code formatting
				dangerouslySetInnerHTML={{
					__html: output,
				}}
			/>
		</LogLine>
	);
};
