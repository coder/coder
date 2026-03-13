import { MockTask } from "testHelpers/entities";
import { renderWithRouter } from "testHelpers/renderHelpers";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Task } from "api/typesGenerated";
import { createMemoryRouter } from "react-router";
import { TasksTable } from "./TasksTable";

const renderTasksTable = (task: Task) => {
	const onCheckChange = vi.fn();
	const router = createMemoryRouter(
		[
			{
				path: "/tasks",
				element: (
					<TasksTable
						tasks={[task]}
						error={undefined}
						onRetry={vi.fn()}
						checkedTaskIds={new Set()}
						onCheckChange={onCheckChange}
					/>
				),
			},
			{
				path: "/tasks/:owner/:taskId",
				element: <div>Task details page</div>,
			},
		],
		{ initialEntries: ["/tasks"] },
	);

	return {
		...renderWithRouter(router),
		onCheckChange,
	};
};

describe("TasksPage", () => {
	it("uses explicit links for primary navigation without row button semantics", async () => {
		const user = userEvent.setup();
		const task: Task = {
			...MockTask,
			id: "task-with-row-link",
		};
		const { onCheckChange, router } = renderTasksTable(task);

		const row = screen.getByTestId(`task-${task.id}`);
		expect(row).not.toHaveAttribute("role");
		expect(row).not.toHaveAttribute("tabindex");

		const primaryLink = within(row).getByRole("link", {
			name: task.display_name,
		});
		expect(primaryLink).toHaveAttribute(
			"href",
			`/tasks/${task.owner_name}/${task.id}`,
		);

		await user.click(screen.getByTestId(`checkbox-${task.id}`));
		expect(onCheckChange).toHaveBeenCalledTimes(1);
		expect(router.state.location.pathname).toBe("/tasks");

		await user.click(
			within(row).getByRole("button", {
				name: /show task actions/i,
			}),
		);
		expect(router.state.location.pathname).toBe("/tasks");

		fireEvent.click(primaryLink);
		await waitFor(() => {
			expect(router.state.location.pathname).toBe(
				`/tasks/${task.owner_name}/${task.id}`,
			);
		});
	});
});
