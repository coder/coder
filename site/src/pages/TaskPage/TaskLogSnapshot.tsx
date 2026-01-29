import { API } from "api/api";
import type { TaskLogEntry } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import type { FC } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";

type TaskLogSnapshotProps = {
	username: string;
	taskId: string;
	actionLabel: string;
	actionHref?: string;
	onAction?: () => void;
};

export const TaskLogSnapshot: FC<TaskLogSnapshotProps> = ({
	username,
	taskId,
	actionLabel,
	actionHref,
	onAction,
}) => {
	const { data, isLoading, error } = useQuery({
		queryKey: ["taskLogs", username, taskId],
		queryFn: () => API.getTaskLogs(username, taskId),
		retry: false,
	});

	if (isLoading) {
		return (
			<div className="w-full max-w-screen-lg mx-auto mt-6">
				<div className="border border-solid border-border rounded-lg h-64 flex items-center justify-center">
					<Loader />
				</div>
			</div>
		);
	}

	if (error) {
		return (
			<div className="w-full max-w-screen-lg mx-auto mt-6">
				<div className="border border-solid border-border rounded-lg h-64 flex items-center justify-center p-6">
					<p className="text-content-secondary text-sm text-center">
						Unable to load conversation history. Please try again.
					</p>
				</div>
			</div>
		);
	}

	const logs = data?.logs ?? [];

	if (logs.length === 0) {
		return (
			<div className="w-full max-w-screen-lg mx-auto mt-6">
				<div className="border border-solid border-border rounded-lg h-64 flex items-center justify-center p-6">
					<p className="text-content-secondary text-sm text-center">
						No conversation history available. The snapshot may not have been
						captured during pause. Restart your task to continue the
						conversation.
					</p>
				</div>
			</div>
		);
	}

	return (
		<div className="w-full max-w-screen-lg mx-auto mt-6">
			<div className="border border-solid border-border rounded-lg overflow-hidden">
				<div className="flex items-center justify-between px-4 py-2 bg-surface-secondary border-b border-solid border-border">
					<span className="text-content-secondary text-sm">
						Last {logs.length} lines of AI chat logs
					</span>
					{actionHref ? (
						<RouterLink
							to={actionHref}
							className="text-content-link text-sm hover:underline"
						>
							{actionLabel}
						</RouterLink>
					) : onAction ? (
						<button
							type="button"
							onClick={onAction}
							className="text-content-link text-sm hover:underline bg-transparent border-none cursor-pointer p-0"
						>
							{actionLabel}
						</button>
					) : (
						<span className="text-content-secondary text-sm">
							{actionLabel}
						</span>
					)}
				</div>
				<ScrollArea className="h-64">
					<div className="p-4 font-mono text-sm whitespace-pre-wrap">
						{logs.map((log, index) => (
							<LogMessage
								key={log.id}
								log={log}
								isLast={index === logs.length - 1}
							/>
						))}
					</div>
				</ScrollArea>
			</div>
		</div>
	);
};

type LogMessageProps = {
	log: TaskLogEntry;
	isLast: boolean;
};

const LogMessage: FC<LogMessageProps> = ({ log, isLast }) => {
	const prefix = log.type === "input" ? "[user]" : "[agent]";

	return (
		<div className={isLast ? "" : "mb-4"}>
			<div className="text-content-secondary">{prefix}</div>
			<div className="text-content-primary">{log.content}</div>
		</div>
	);
};
