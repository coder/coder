import { API, type TasksFilter } from "api/api";
import { templates } from "api/queries/templates";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { useAuthenticated } from "hooks";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { TaskPrompt } from "./TaskPrompt";
import { TasksTable } from "./TasksTable";
import { UsersCombobox } from "./UsersCombobox";

const TasksPage: FC = () => {
	const AITemplatesQuery = useQuery(
		templates({
			q: "has-ai-task:true",
		}),
	);

	// Tasks
	const { user, permissions } = useAuthenticated();
	const userFilter = useSearchParamsKey({
		key: "username",
		defaultValue: user.username,
	});
	const filter: TasksFilter = {
		username: userFilter.value,
	};
	const tasksQuery = useQuery({
		queryKey: ["tasks", filter],
		queryFn: () => API.experimental.getTasks(filter),
		refetchInterval: 10_000,
	});

	return (
		<>
			<Helmet>
				<title>{pageTitle("AI Tasks")}</title>
			</Helmet>
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
						templates={AITemplatesQuery.data}
						error={AITemplatesQuery.error}
						onRetry={AITemplatesQuery.refetch}
					/>
					{AITemplatesQuery.isSuccess && (
						<section>
							{permissions.viewDeploymentConfig && (
								<TasksControls
									username={userFilter.value}
									onUsernameChange={(username) =>
										// When selecting a selected user, clear the filter
										userFilter.setValue(
											username === userFilter.value ? "" : username,
										)
									}
								/>
							)}
							<TasksTable
								tasks={tasksQuery.data}
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

type TasksControlsProps = {
	username: string;
	onUsernameChange: (username: string) => void;
};

const TasksControls: FC<TasksControlsProps> = ({
	username,
	onUsernameChange,
}) => {
	return (
		<section className="mt-6" aria-labelledby="filters-title">
			<h3 id="filters-title" className="sr-only">
				Filters
			</h3>
			<UsersCombobox value={username} onValueChange={onUsernameChange} />
		</section>
	);
};

export default TasksPage;
