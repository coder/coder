import { API, type TasksFilter } from "api/api";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { templates } from "api/queries/templates";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { useAuthenticated } from "hooks";
import { type FC, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { type UserOption, UsersCombobox } from "./UsersCombobox";
import { TasksTable } from "./TasksTable";
import { TaskPrompt } from "./TaskPrompt";

const TasksPage: FC = () => {
	const AITemplatesQuery = useQuery(
		templates({
			q: "has-ai-task:true",
		}),
	);

	// Tasks
	const { user, permissions } = useAuthenticated();
	const [userOption, setUserOption] = useState<UserOption | undefined>({
		value: user.username,
		label: user.name || user.username,
		avatarUrl: user.avatar_url,
	});
	const filter: TasksFilter = {
		username: userOption?.value,
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
									userOption={userOption}
									onUserOptionChange={setUserOption}
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
	userOption: UserOption | undefined;
	onUserOptionChange: (userOption: UserOption | undefined) => void;
};

const TasksControls: FC<TasksControlsProps> = ({
	userOption,
	onUserOptionChange,
}) => {
	return (
		<section className="mt-6" aria-labelledby="filters-title">
			<h3 id="filters-title" className="sr-only">
				Filters
			</h3>
			<UsersCombobox
				selectedOption={userOption}
				onSelect={onUserOptionChange}
			/>
		</section>
	);
};

export default TasksPage;
