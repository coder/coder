import dayjs from "dayjs";
import {
	type FC,
	Fragment,
	type HTMLAttributes,
	useLayoutEffect,
	useRef,
} from "react";
import type { ProvisionerJobLog, WorkspaceBuild } from "#/api/typesGenerated";
import type { Line } from "#/components/Logs/LogLine";
import { DEFAULT_LOG_LINE_SIDE_PADDING, Logs } from "#/components/Logs/Logs";
import { cn } from "#/utils/cn";

type Stage = ProvisionerJobLog["stage"];
type LogsGroupedByStage = Record<Stage, ProvisionerJobLog[]>;
type GroupLogsByStageFn = (logs: ProvisionerJobLog[]) => LogsGroupedByStage;

const groupLogsByStage: GroupLogsByStageFn = (logs) => {
	const logsByStage: LogsGroupedByStage = {};

	for (const log of logs) {
		if (log.stage in logsByStage) {
			logsByStage[log.stage].push(log);
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

	const startedAt = dayjs(logs[0].created_at);
	const completedAt = dayjs(logs[logs.length - 1].created_at);
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
			className={cn("font-mono border border-border rounded-lg", className)}
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
							className={cn(
								"logs-header",
								"flex items-center border-solid border-0 border-b border-border font-sans",
								"bg-surface-primary text-xs font-semibold leading-none",
								"first-of-type:pt-4",
							)}
							style={{
								padding: `12px var(--log-line-side-padding, ${DEFAULT_LOG_LINE_SIDE_PADDING}px)`,
							}}
						>
							<div>{stage}</div>
							{shouldDisplayDuration && (
								<div className="ml-auto text-xs text-content-secondary">
									{duration} seconds
								</div>
							)}
						</div>
						{!isEmpty && (
							<Logs
								className="border-b-border"
								hideTimestamps={hideTimestamps}
								lines={lines}
							/>
						)}
					</Fragment>
				);
			})}
		</div>
	);
};
