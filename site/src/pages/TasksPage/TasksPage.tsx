import { API } from "api/api";
import { templates } from "api/queries/templates";
import type { TasksFilter } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Button, type ButtonProps } from "components/Button/Button";
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
	const ownerFilter = useSearchParamsKey({
		key: "owner",
		defaultValue: user.username,
	});
	const tab = useSearchParamsKey({
		key: "tab",
		defaultValue: "all",
	});
	const filter: TasksFilter = {
		owner: ownerFilter.value,
	};
	const tasksQuery = useQuery({
		queryKey: ["tasks", filter],
		queryFn: () => API.tasks.getTasks(filter),
		refetchInterval: 10_000,
	});
	const idleTasks = tasksQuery.data?.filter(
		(task) => task.current_state?.state === "idle",
	);
	const displayedTasks =
		tab.value === "waiting-for-input" ? idleTasks : tasksQuery.data;

	return (
		<>
			<title>{pageTitle("AI Tasks")}</title>
			<Margins>
				<PageHeader>
					<PageHeaderTitle>Tasks</PageHeaderTitle>
					<PageHeaderSubtitle>Automate tasks with AI</PageHeaderSubtitle>
				</PageHeader>

				<main className="pb-8">
					<TaskPrompt
						templates={aiTemplatesQuery.data}
						error={aiTemplatesQuery.error}
						onRetry={aiTemplatesQuery.refetch}
					/>
					{aiTemplatesQuery.isSuccess &&
						aiTemplatesQuery.data &&
						aiTemplatesQuery.data.length > 0 && (
							<section className="py-8">
								{permissions.viewDeploymentConfig && (
									<section
										className="mt-6 flex justify-between"
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
											value={ownerFilter.value}
											onValueChange={(username) => {
												ownerFilter.setValue(
													username === ownerFilter.value ? "" : username,
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
