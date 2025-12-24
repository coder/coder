import type { Interpolation, Theme } from "@emotion/react";
import type { ProvisionerJobLog, WorkspaceBuild } from "api/typesGenerated";
import type { Line } from "components/Logs/LogLine";
import { DEFAULT_LOG_LINE_SIDE_PADDING, Logs } from "components/Logs/Logs";
import dayjs from "dayjs";
import {
	type FC,
	Fragment,
	type HTMLAttributes,
	useLayoutEffect,
	useRef,
} from "react";
import { BODY_FONT_FAMILY } from "theme/constants";

const Language = {
	seconds: "seconds",
};

type Stage = ProvisionerJobLog["stage"];
type LogsGroupedByStage = Record<Stage, ProvisionerJobLog[]>;
type GroupLogsByStageFn = (logs: ProvisionerJobLog[]) => LogsGroupedByStage;

const groupLogsByStage: GroupLogsByStageFn = (logs) => {
	const logsByStage: LogsGroupedByStage = {};

	for (const log of logs) {
		if (log.stage in logsByStage) {
			logsByStage[log.stage]!.push(log);
		} else {
			logsByStage[log.stage] = [log];
		}
	}

	return logsByStage;
};

const getStageDurationInSeconds = (logs: ProvisionerJobLog[]) => {
	if (logs.length < 2) {
		return;
	}

	const startedAt = dayjs(logs[0]!.created_at);
	const completedAt = dayjs(logs[logs.length - 1]!.created_at);
	return completedAt.diff(startedAt, "seconds");
};

interface WorkspaceBuildLogsProps extends HTMLAttributes<HTMLDivElement> {
	hideTimestamps?: boolean;
	sticky?: boolean;
	logs: ProvisionerJobLog[];
	build?: WorkspaceBuild;
	disableAutoscroll?: boolean;
}

export const WorkspaceBuildLogs: FC<WorkspaceBuildLogsProps> = ({
	hideTimestamps,
	sticky,
	logs,
	build,
	disableAutoscroll,
	className,
	...attrs
}) => {
	const groupedLogsByStage = groupLogsByStage(logs);

	const ref = useRef<HTMLDivElement>(null);
	useLayoutEffect(() => {
		if (disableAutoscroll || logs.length === 0) {
			return;
		}
		ref.current?.scrollIntoView({ block: "end" });
	}, [logs, disableAutoscroll]);

	return (
		<div
			ref={ref}
			className="font-mono border border-border rounded-lg"
			{...attrs}
		>
			{Object.entries(groupedLogsByStage).map(([stage, logs]) => {
				const isEmpty = logs.every((log) => log.output === "");
				const lines = logs.map<Line>((log) => ({
					id: log.id,
					time: log.created_at,
					output: log.output,
					level: log.log_level,
					sourceId: log.log_source,
				}));
				const duration = getStageDurationInSeconds(logs);
				const shouldDisplayDuration = duration !== undefined;

				return (
					<Fragment key={stage}>
						<div
							css={[styles.header, sticky && styles.sticky]}
							className="logs-header"
						>
							<div>{stage}</div>
							{shouldDisplayDuration && (
								<div css={styles.duration}>
									{duration} {Language.seconds}
								</div>
							)}
						</div>
						{!isEmpty && <Logs hideTimestamps={hideTimestamps} lines={lines} />}
					</Fragment>
				);
			})}
		</div>
	);
};

const styles = {
	header: (theme) => ({
		fontSize: 13,
		fontWeight: 600,
		padding: `12px var(--log-line-side-padding, ${DEFAULT_LOG_LINE_SIDE_PADDING}px)`,
		display: "flex",
		alignItems: "center",
		fontFamily: BODY_FONT_FAMILY,
		borderBottom: `1px solid ${theme.palette.divider}`,
		background: theme.palette.background.default,
		lineHeight: "1",

		"&:last-child": {
			borderBottom: 0,
			borderRadius: "0 0 8px 8px",
		},

		"&:first-of-type": {
			borderRadius: "8px 8px 0 0",
		},
	}),

	sticky: {
		position: "sticky",
		top: 0,
	},

	duration: (theme) => ({
		marginLeft: "auto",
		color: theme.palette.text.secondary,
		fontSize: 12,
	}),
} satisfies Record<string, Interpolation<Theme>>;
