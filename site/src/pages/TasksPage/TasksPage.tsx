import { API, type TasksFilter } from "api/api";
import { templates } from "api/queries/templates";
import { Badge } from "components/Badge/Badge";
import { Button, type ButtonProps } from "components/Button/Button";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { useAuthenticated } from "hooks";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { TaskPrompt } from "modules/tasks/TaskPrompt/TaskPrompt";
import type { FC } from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { TasksTable } from "./TasksTable";
import { UsersCombobox } from "./UsersCombobox";

const TasksPage: FC = () => {
	const aiTemplatesQuery = useQuery(
		templates({
			q: "has-ai-task:true",
		}),
	);

	const { user, permissions } = useAuthenticated();
	const userFilter = useSearchParamsKey({
		key: "username",
		defaultValue: user.username,
	});
	const tab = useSearchParamsKey({
		key: "tab",
		defaultValue: "all",
	});
	const filter: TasksFilter = {
		username: userFilter.value,
	};
	const tasksQuery = useQuery({
		queryKey: ["tasks", filter],
		queryFn: () => API.experimental.getTasks(filter),
		refetchInterval: 10_000,
		// TODO: Switch to sorting by latest_status_app.created_at once it’s reliable.
		// Currently, it doesn’t always update fast enough for a good UX, so we’re
		// temporarily sorting by workspace.created_at instead.
		select: (tasks) =>
			tasks.toSorted(
				(a, b) =>
					new Date(b.workspace.created_at).getTime() -
					new Date(a.workspace.created_at).getTime(),
			),
	});
	const idleTasks = tasksQuery.data?.filter(
		(task) => task.workspace.latest_app_status?.state === "idle",
	);
	const displayedTasks =
		tab.value === "waiting-for-input" ? idleTasks : tasksQuery.data;

	return (
		<>
			<title>{pageTitle("AI Tasks")}</title>
			<Margins>
				<PageHeader>
					<span className="flex flex-row gap-2">
						<PageHeaderTitle>Tasks</PageHeaderTitle>
						<FeatureStageBadge contentType={"beta"} size="md" />
					</span>
					<PageHeaderSubtitle>Automate tasks with AI</PageHeaderSubtitle>
				</PageHeader>

				<main className="pb-8">
					<TaskPrompt
						templates={aiTemplatesQuery.data}
						error={aiTemplatesQuery.error}
						onRetry={aiTemplatesQuery.refetch}
					/>
					{aiTemplatesQuery.isSuccess && (
						<section>
							{permissions.viewDeploymentConfig && (
								<section
									className="mt-6 pt-6 border-t border-border flex justify-between"
									aria-label="Controls"
								>
									<div className="flex items-center bg-surface-secondary rounded p-1">
										<PillButton
											active={tab.value === "all"}
											onClick={() => tab.setValue("all")}
										>
											All tasks
										</PillButton>
										<PillButton
											disabled={!idleTasks || idleTasks.length === 0}
											active={tab.value === "waiting-for-input"}
											onClick={() => tab.setValue("waiting-for-input")}
										>
											Waiting for input
											{idleTasks && idleTasks.length > 0 && (
												<Badge className="-mr-0.5" size="xs" variant="info">
													{idleTasks.length}
												</Badge>
											)}
										</PillButton>
									</div>

									<UsersCombobox
										value={userFilter.value}
										onValueChange={(username) => {
											userFilter.setValue(
												username === userFilter.value ? "" : username,
											);
										}}
									/>
								</section>
							)}

							<TasksTable
								tasks={displayedTasks}
								error={tasksQuery.error}
								onRetry={tasksQuery.refetch}
							/>
						</section>
					)}
				</main>
			</Margins>
		</>
	);
};

type PillButtonProps = ButtonProps & {
	active?: boolean;
};

const PillButton: FC<PillButtonProps> = ({ className, active, ...props }) => {
	return (
		<Button
			size="sm"
			className={cn([
				className,
				"border-0 gap-2",
				{
					"bg-surface-primary hover:bg-surface-primary": active,
				},
			])}
			variant={active ? "outline" : "subtle"}
			{...props}
		/>
	);
};

export default TasksPage;
