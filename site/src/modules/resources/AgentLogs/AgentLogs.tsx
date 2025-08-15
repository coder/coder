import MuiTooltip from "@mui/material/Tooltip";
import type { WorkspaceAgentLogSource } from "api/typesGenerated";
import type { Line } from "components/Logs/LogLine";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { type ComponentProps, forwardRef } from "react";
import { FixedSizeList as List } from "react-window";
import { cn } from "utils/cn";
import { AGENT_LOG_LINE_HEIGHT, AgentLogLine } from "./AgentLogLine";
import { Info } from "lucide-react";
import { Badge } from "components/Badge/Badge";

const fallbackLog: WorkspaceAgentLogSource = {
	created_at: "",
	display_name: "Logs",
	icon: "",
	id: "00000000-0000-0000-0000-000000000000",
	workspace_agent_id: "",
};

function groupLogSourcesById(
	sources: readonly WorkspaceAgentLogSource[],
): Record<string, WorkspaceAgentLogSource> {
	const sourcesById: Record<string, WorkspaceAgentLogSource> = {};
	for (const source of sources) {
		sourcesById[source.id] = source;
	}
	return sourcesById;
}

type AgentLogsProps = Omit<
	ComponentProps<typeof List>,
	"children" | "itemSize" | "itemCount" | "itemKey"
> & {
	logs: readonly Line[];
	sources: readonly WorkspaceAgentLogSource[];
	overflowed: boolean;
};

export const AgentLogs = forwardRef<List, AgentLogsProps>(
	({ logs, sources, overflowed, ...listProps }, ref) => {
		// getLogSource must always returns a valid log source. We need this to
		// support deployments that were made before `coder_script` was created
		// and that haven't updated to a newer Coder version yet
		const logSourceByID = groupLogSourcesById(sources);
		const getLogSource = (id: string) => logSourceByID[id] || fallbackLog;

		return (
			<div className="flex flex-col bg-surface-secondary">
				{overflowed && (
					<TooltipProvider delayDuration={100}>
						<Tooltip>
							<TooltipTrigger asChild className="max-w-fit pt-4 px-4">
								<span>
									<Badge variant="warning">Logs overflowed</Badge>
								</span>
							</TooltipTrigger>
							<TooltipContent asChild className="w-full text-sm text-blue-500 text-content-secondary bg-surface-primary max-w-prose leading-relaxed m-0 p-4">
								<p>
									Startup logs exceeded the max size of{" "}
									<span className="tracking-wide font-mono">1MB</span>, and
									will not continue to be written to the database! Logs will
									continue to be written to the{" "}
									<span className="font-mono bg-surface-tertiary rounded-md px-1.5 py-0.5">
										/tmp/coder-startup-script.log
									</span>{" "}
									file in the workspace.
								</p>
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				)}

				<List
					{...listProps}
					ref={ref}
					// We need the div selector to be able to apply the padding
					// top from startupLogs
					className="pt-4 [&>div]:relative bg-surface-secondary"
					itemCount={logs.length}
					itemSize={AGENT_LOG_LINE_HEIGHT}
					itemKey={(index) => logs[index].id || 0}
				>
					{({ index, style }) => {
						const log = logs[index];
						const logSource = getLogSource(log.sourceId);

						let assignedIcon = false;
						let icon: JSX.Element;
						// If no icon is specified, we show a deterministic
						// colored circle to identify unique scripts.
						if (logSource.icon) {
							icon = (
								<img
									src={logSource.icon}
									alt=""
									className="size-3.5 mr-2 shrink-0"
								/>
							);
						} else {
							icon = (
								<div
									role="presentation"
									className="size-3.5 mr-2 shrink-0 rounded-full"
									style={{
										background: determineScriptDisplayColor(
											logSource.display_name,
										),
									}}
								/>
							);
							assignedIcon = true;
						}

						const doesNextLineHaveDifferentSource =
							index < logs.length - 1 &&
							getLogSource(logs[index + 1].sourceId).id !== log.sourceId;

						// We don't want every line to repeat the icon, because
						// that is ugly and repetitive. This removes the icon
						// for subsequent lines of the same source and shows a
						// line instead, visually indicating they are from the
						// same source.
						const shouldHideSource =
							index > 0 &&
							getLogSource(logs[index - 1].sourceId).id === log.sourceId;
						if (shouldHideSource) {
							icon = (
								<div className="size-3.5 mr-2 flex justify-center relative shrink-0">
									<div
										// dashed-line class comes from LogLine
										className={cn(
											"dashed-line w-0.5 rounded-[2px] bg-surface-tertiary h-full",
											doesNextLineHaveDifferentSource && "h-1/2",
										)}
									/>
									{doesNextLineHaveDifferentSource && (
										<div
											role="presentation"
											className="dashed-line h-[2px] w-1/2 top-[calc(50%-2px)] left-[calc(50%-1px)] rounded-[2px] absolute bg-surface-tertiary"
										/>
									)}
								</div>
							);
						}

						return (
							<AgentLogLine
								line={log}
								number={index + 1}
								maxLineNumber={logs.length}
								style={style}
								sourceIcon={
									<MuiTooltip
										title={
											<>
												{logSource.display_name}
												{assignedIcon && (
													<i>
														<br />
														No icon specified!
													</i>
												)}
											</>
										}
									>
										{icon}
									</MuiTooltip>
								}
							/>
						);
					}}
				</List>
			</div>
		);
	},
);

// These colors were picked at random. Feel free
// to add more, adjust, or change! Users will not
// depend on these colors.
const scriptDisplayColors: readonly string[] = [
	"#85A3B2",
	"#A37EB2",
	"#C29FDE",
	"#90B3D7",
	"#829AC7",
	"#728B8E",
	"#506080",
	"#5654B0",
	"#6B56D6",
	"#7847CC",
];

const determineScriptDisplayColor = (displayName: string): string => {
	const hash = displayName.split("").reduce((hash, char) => {
		return (hash << 5) + hash + char.charCodeAt(0); // bit-shift and add for our simple hash
	}, 0);
	return scriptDisplayColors[Math.abs(hash) % scriptDisplayColors.length];
};
