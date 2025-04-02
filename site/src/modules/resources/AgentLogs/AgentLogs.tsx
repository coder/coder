import type { Interpolation, Theme } from "@emotion/react";
import Tooltip from "@mui/material/Tooltip";
import type { WorkspaceAgentLogSource } from "api/typesGenerated";
import type { Line } from "components/Logs/LogLine";
import { type ComponentProps, forwardRef, useMemo } from "react";
import { FixedSizeList as List } from "react-window";
import { AGENT_LOG_LINE_HEIGHT, AgentLogLine } from "./AgentLogLine";

type AgentLogsProps = Omit<
	ComponentProps<typeof List>,
	"children" | "itemSize" | "itemCount"
> & {
	logs: readonly Line[];
	sources: readonly WorkspaceAgentLogSource[];
};

export const AgentLogs = forwardRef<List, AgentLogsProps>(
	({ logs, sources, ...listProps }, ref) => {
		const logSourceByID = useMemo(() => {
			const sourcesById: { [id: string]: WorkspaceAgentLogSource } = {};
			for (const source of sources) {
				sourcesById[source.id] = source;
			}
			return sourcesById;
		}, [sources]);

		return (
			<List
				ref={ref}
				css={styles.logs}
				itemCount={logs.length}
				itemSize={AGENT_LOG_LINE_HEIGHT}
				{...listProps}
			>
				{({ index, style }) => {
					const log = logs[index];
					// getLogSource always returns a valid log source.
					// This is necessary to support deployments before `coder_script`.
					// Existed that haven't restarted their agents.
					const getLogSource = (id: string): WorkspaceAgentLogSource => {
						return (
							logSourceByID[id] || {
								created_at: "",
								display_name: "Logs",
								icon: "",
								id: "00000000-0000-0000-0000-000000000000",
								workspace_agent_id: "",
							}
						);
					};
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
								width={14}
								height={14}
								css={{
									marginRight: 8,
									flexShrink: 0,
								}}
							/>
						);
					} else {
						icon = (
							<div
								css={{
									width: 14,
									height: 14,
									marginRight: 8,
									flexShrink: 0,
									background: determineScriptDisplayColor(
										logSource.display_name,
									),
									borderRadius: "100%",
								}}
							/>
						);
						assignedIcon = true;
					}

					let nextChangesSource = false;
					if (index < logs.length - 1) {
						nextChangesSource =
							getLogSource(logs[index + 1].sourceId).id !== log.sourceId;
					}
					// We don't want every line to repeat the icon, because
					// that is ugly and repetitive. This removes the icon
					// for subsequent lines of the same source and shows a
					// line instead, visually indicating they are from the
					// same source.
					if (
						index > 0 &&
						getLogSource(logs[index - 1].sourceId).id === log.sourceId
					) {
						icon = (
							<div
								css={{
									width: 14,
									height: 14,
									marginRight: 8,
									display: "flex",
									justifyContent: "center",
									position: "relative",
									flexShrink: 0,
								}}
							>
								<div
									className="dashed-line"
									css={(theme) => ({
										height: nextChangesSource ? "50%" : "100%",
										width: 2,
										background: theme.experimental.l1.outline,
										borderRadius: 2,
									})}
								/>
								{nextChangesSource && (
									<div
										className="dashed-line"
										css={(theme) => ({
											height: 2,
											width: "50%",
											top: "calc(50% - 2px)",
											left: "calc(50% - 1px)",
											background: theme.experimental.l1.outline,
											borderRadius: 2,
											position: "absolute",
										})}
									/>
								)}
							</div>
						);
					}

					return (
						<AgentLogLine
							line={logs[index]}
							number={index + 1}
							maxLineNumber={logs.length}
							style={style}
							sourceIcon={
								<Tooltip
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
								</Tooltip>
							}
						/>
					);
				}}
			</List>
		);
	},
);

// These colors were picked at random. Feel free
// to add more, adjust, or change! Users will not
// depend on these colors.
const scriptDisplayColors = [
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

const styles = {
	logs: (theme) => ({
		backgroundColor: theme.palette.background.paper,
		paddingTop: 16,

		// We need this to be able to apply the padding top from startupLogs
		"& > div": {
			position: "relative",
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
