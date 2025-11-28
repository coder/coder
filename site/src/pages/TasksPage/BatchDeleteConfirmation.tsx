import type { Task } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { ClockIcon, UserIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";

dayjs.extend(relativeTime);

type BatchDeleteConfirmationProps = {
	checkedTasks: readonly Task[];
	workspaceCount: number;
	open: boolean;
	isLoading: boolean;
	onClose: () => void;
	onConfirm: () => void;
};

export const BatchDeleteConfirmation: FC<BatchDeleteConfirmationProps> = ({
	checkedTasks,
	workspaceCount,
	open,
	onClose,
	onConfirm,
	isLoading,
}) => {
	const [stage, setStage] = useState<"consequences" | "tasks">("consequences");

	const onProceed = () => {
		switch (stage) {
			case "tasks":
				onConfirm();
				break;
			case "consequences":
				setStage("tasks");
				break;
		}
	};

	const taskCount = `${checkedTasks.length} ${
		checkedTasks.length === 1 ? "task" : "tasks"
	}`;
	const workspaceCountText = `${workspaceCount} ${
		workspaceCount === 1 ? "workspace" : "workspaces"
	}`;

	let confirmText: ReactNode = <>Review selected tasks&hellip;</>;
	if (stage === "tasks") {
		confirmText = (
			<>
				Delete {taskCount} and {workspaceCountText}
			</>
		);
	}

	return (
		<ConfirmDialog
			type="delete"
			open={open}
			onClose={() => {
				setStage("consequences");
				onClose();
			}}
			title={`Delete ${taskCount}`}
			confirmLoading={isLoading}
			confirmText={confirmText}
			onConfirm={onProceed}
			description={
				<>
					{stage === "consequences" && <Consequences />}
					{stage === "tasks" && <Tasks tasks={checkedTasks} />}
				</>
			}
		/>
	);
};

interface TasksStageProps {
	tasks: readonly Task[];
}

const Consequences: FC = () => {
	return (
		<>
			<p>Deleting tasks is irreversible!</p>
			<ul className="flex flex-col gap-2 pl-4 mb-0">
				<li>
					Tasks with associated workspaces will have those workspaces deleted.
				</li>
				<li>Any data stored in task workspaces will be permanently deleted.</li>
			</ul>
		</>
	);
};

const Tasks: FC<TasksStageProps> = ({ tasks }) => {
	const mostRecent = tasks.reduce(
		(latestSoFar, against) => {
			if (!latestSoFar) {
				return against;
			}

			return new Date(against.updated_at).getTime() >
				new Date(latestSoFar.updated_at).getTime()
				? against
				: latestSoFar;
		},
		undefined as Task | undefined,
	);

	const ownersCount = new Set(tasks.map((it) => it.owner_name)).size;
	const ownersCountDisplay = `${ownersCount} ${ownersCount === 1 ? "owner" : "owners"}`;

	return (
		<>
			<ul className="list-none p-0 border border-solid border-zinc-200 dark:border-zinc-700 rounded-lg overflow-x-hidden overflow-y-auto max-h-[184px]">
				{tasks.map((task) => (
					<li
						key={task.id}
						className="py-2 px-4 border-solid border-0 border-b border-zinc-200 dark:border-zinc-700 last:border-b-0"
					>
						<div className="flex items-center justify-between gap-6">
							<span className="font-medium text-content-primary max-w-[400px] overflow-hidden text-ellipsis whitespace-nowrap">
								{task.display_name}
							</span>

							<div className="flex flex-col text-sm items-end">
								<div className="flex items-center gap-2">
									<span className="whitespace-nowrap">{task.owner_name}</span>
									<UserIcon className="size-icon-sm -m-px" />
								</div>
								<div className="flex items-center gap-2">
									<span className="whitespace-nowrap">
										{dayjs(task.updated_at).fromNow()}
									</span>
									<ClockIcon className="size-icon-xs" />
								</div>
							</div>
						</div>
					</li>
				))}
			</ul>
			<div className="flex flex-wrap justify-center gap-x-5 gap-y-1.5 text-sm">
				<div className="flex items-center gap-2">
					<UserIcon className="size-icon-sm -m-px" />
					<span>{ownersCountDisplay}</span>
				</div>
				{mostRecent && (
					<div className="flex items-center gap-2">
						<ClockIcon className="size-icon-xs" />
						<span>Last updated {dayjs(mostRecent.updated_at).fromNow()}</span>
					</div>
				)}
			</div>
		</>
	);
};
