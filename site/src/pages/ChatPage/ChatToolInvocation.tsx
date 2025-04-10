import { FC } from "react";
import type { ToolInvocation } from "@ai-sdk/ui-utils";
import { useTheme } from "@emotion/react";

interface ChatToolInvocationProps {
	toolInvocation: ToolInvocation;
}

export const ChatToolInvocation: FC<ChatToolInvocationProps> = ({
	toolInvocation,
}) => {
	const theme = useTheme();
	return (
		<div
			css={{
				marginTop: theme.spacing(1),
				marginLeft: theme.spacing(2),
				borderLeft: `2px solid ${theme.palette.info.light}`,
				paddingLeft: theme.spacing(1.5),
				fontSize: "0.875em",
				fontFamily: "monospace",
			}}
		>
			<div
				css={{
					color: theme.palette.info.light,
					fontStyle: "italic",
					fontWeight: 500,
					marginBottom: theme.spacing(0.5),
				}}
			>
				🛠️ Tool Call: {toolInvocation.toolName}
			</div>
			<div
				css={{
					backgroundColor: theme.palette.action.hover,
					padding: theme.spacing(1.5),
					borderRadius: "6px",
					marginTop: theme.spacing(0.5),
					color: theme.palette.text.secondary,
				}}
			>
				<div css={{ marginBottom: theme.spacing(1) }}>
					Arguments:
					<div
						css={{
							marginTop: theme.spacing(0.5),
							fontFamily: "monospace",
							whiteSpace: "pre-wrap",
							wordBreak: "break-all",
							fontSize: "0.9em",
							color: theme.palette.text.primary,
						}}
					>
						{JSON.stringify(toolInvocation.args, null, 2)}
					</div>
				</div>
				{"result" in toolInvocation && (
					<div>
						Result:
						<div
							css={{
								marginTop: theme.spacing(0.5),
								fontFamily: "monospace",
								whiteSpace: "pre-wrap",
								wordBreak: "break-all",
								fontSize: "0.9em",
								color: theme.palette.text.primary,
							}}
						>
							{JSON.stringify(toolInvocation.result, null, 2)}
						</div>
					</div>
				)}
			</div>
		</div>
	);
};
