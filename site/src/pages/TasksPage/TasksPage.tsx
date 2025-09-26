import { templates } from "api/queries/templates";
import type { FC } from "react";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { TaskPrompt } from "./TaskPrompt";

const TasksPage: FC = () => {
	const aiTemplatesQuery = useQuery(
		templates({
			q: "has-ai-task:true",
		}),
	);

	return (
		<>
			<title>{pageTitle("AI Tasks")}</title>

			<main className="p-6 flex items-center justify-center h-full">
				<div className="w-full max-w-2xl">
					<h1 className="text-center m-0 pb-10 font-medium">
						What do you want to get done today?
					</h1>
					<TaskPrompt
						templates={aiTemplatesQuery.data}
						error={aiTemplatesQuery.error}
						onRetry={aiTemplatesQuery.refetch}
					/>
				</div>
			</main>
		</>
	);
};

export default TasksPage;
