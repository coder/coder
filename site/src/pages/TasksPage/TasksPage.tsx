import { API } from "api/api";
import { templates } from "api/queries/templates";

import type { TasksFilter } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Button, type ButtonProps } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Spinner } from "components/Spinner/Spinner";
import { TableToolbar } from "components/TableToolbar/TableToolbar";
import { useAuthenticated } from "hooks";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { ChevronDownIcon, TrashIcon } from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { TaskPrompt } from "modules/tasks/TaskPrompt/TaskPrompt";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { BatchDeleteConfirmation } from "./BatchDeleteConfirmation";
import { useBatchTaskActions } from "./batchActions";
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
		queryFn: () => API.getTasks(filter),
		refetchInterval: 10_000,
	});
	const idleTasks = tasksQuery.data?.filter(
		(task) => task.current_state?.state === "idle",
	);
	const displayedTasks =
		tab.value === "waiting-for-input" ? idleTasks : tasksQuery.data;

	const [checkedTaskIds, setCheckedTaskIds] = useState<Set<string>>(new Set());
	const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);

	const checkedTasks =
		displayedTasks?.filter((t) => checkedTaskIds.has(t.id)) ?? [];

	const batchActions = useBatchTaskActions({
		onSuccess: async () => {
			await tasksQuery.refetch();
			setCheckedTaskIds(new Set());
			setIsDeleteDialogOpen(false);
		},
	});

	const handleCheckChange = (newIds: Set<string>) => {
		setCheckedTaskIds(newIds);
	};

	const handleBatchDelete = () => {
		setIsDeleteDialogOpen(true);
	};

	const handleConfirmDelete = async () => {
		await batchActions.delete(checkedTasks);
	};

	const { entitlements } = useDashboard();
	const canCheckTasks = entitlements.features.task_batch_actions.enabled;

	// Count workspaces that will be deleted with the selected tasks.
	const workspaceCount = checkedTasks.filter(
		(t) => t.workspace_id !== null,
	).length;

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
												onClick={() => {
													tab.setValue("all");
													setCheckedTaskIds(new Set());
												}}
											>
												All tasks
											</PillButton>
											<PillButton
												disabled={!idleTasks || idleTasks.length === 0}
												active={tab.value === "waiting-for-input"}
												onClick={() => {
													tab.setValue("waiting-for-input");
													setCheckedTaskIds(new Set());
												}}
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
												setCheckedTaskIds(new Set());
											}}
										/>
									</section>
								)}

								<div className="mt-6">
									<TableToolbar>
										{checkedTasks.length > 0 ? (
											<>
												<div>
													Selected <strong>{checkedTasks.length}</strong> of{" "}
													<strong>{displayedTasks?.length}</strong>{" "}
													{displayedTasks?.length === 1 ? "task" : "tasks"}
												</div>

												<DropdownMenu>
													<DropdownMenuTrigger asChild>
														<Button
															disabled={batchActions.isProcessing}
															variant="outline"
															size="sm"
															className="ml-auto"
														>
															Bulk actions
															<Spinner loading={batchActions.isProcessing}>
																<ChevronDownIcon className="size-4" />
															</Spinner>
														</Button>
													</DropdownMenuTrigger>
													<DropdownMenuContent align="end">
														<DropdownMenuItem
															className="text-content-destructive focus:text-content-destructive"
															onClick={handleBatchDelete}
														>
															<TrashIcon /> Delete&hellip;
														</DropdownMenuItem>
													</DropdownMenuContent>
												</DropdownMenu>
											</>
										) : (
											<div>
												Showing{" "}
												{displayedTasks && displayedTasks.length > 0 ? (
													<>
														<strong>1</strong> to{" "}
														<strong>{displayedTasks.length}</strong> of{" "}
														<strong>{displayedTasks.length}</strong>
													</>
												) : (
													<strong>0</strong>
												)}{" "}
												{displayedTasks?.length === 1 ? "task" : "tasks"}
											</div>
										)}
									</TableToolbar>
								</div>

								<TasksTable
									tasks={displayedTasks}
									error={tasksQuery.error}
									onRetry={tasksQuery.refetch}
									checkedTaskIds={checkedTaskIds}
									onCheckChange={handleCheckChange}
									canCheckTasks={canCheckTasks}
								/>
							</section>
						)}
				</main>

				<BatchDeleteConfirmation
					open={isDeleteDialogOpen}
					checkedTasks={checkedTasks}
					workspaceCount={workspaceCount}
					isLoading={batchActions.isProcessing}
					onClose={() => setIsDeleteDialogOpen(false)}
					onConfirm={handleConfirmDelete}
				/>
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
