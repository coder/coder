import { MockTask, MockTasks } from "testHelpers/entities";
import { render } from "testHelpers/renderHelpers";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import { describe, expect, it, vi } from "vitest";
import { TasksTable } from "./TasksTable";

describe("TasksTable", () => {
	it("shows pause button for active tasks, resume for paused/error", async () => {
		const mixedTasks = [
			{ ...MockTask, id: "active-task", status: "active" as const },
			{ ...MockTask, id: "paused-task", status: "paused" as const },
			{ ...MockTask, id: "error-task", status: "error" as const },
		];
		render(
			<TasksTable tasks={mixedTasks} error={undefined} onRetry={() => {}} />,
		);

		await screen.findByRole("table");

		expect(screen.getAllByRole("button", { name: /pause task/i })).toHaveLength(
			1,
		);
		expect(
			screen.getAllByRole("button", { name: /resume task/i }),
		).toHaveLength(2);
	});

	it("shows disabled pause button for pending/initializing tasks", async () => {
		const pendingTasks = [{ ...MockTask, status: "pending" as const }];
		render(
			<TasksTable tasks={pendingTasks} error={undefined} onRetry={() => {}} />,
		);

		const pauseButton = await screen.findByRole("button", {
			name: /pause task/i,
		});
		expect(pauseButton).toBeDisabled();
	});

	it("hides buttons for unknown status", async () => {
		const unknownTasks = [{ ...MockTask, status: "unknown" as const }];
		render(
			<TasksTable tasks={unknownTasks} error={undefined} onRetry={() => {}} />,
		);

		await screen.findByRole("table");

		expect(
			screen.queryByRole("button", { name: /pause task/i }),
		).not.toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: /resume task/i }),
		).not.toBeInTheDocument();
	});

	it("calls stopWorkspace on pause click", async () => {
		const user = userEvent.setup();
		const stopWorkspaceSpy = vi
			.spyOn(API, "stopWorkspace")
			.mockResolvedValue({} as never);

		const activeTasks = [{ ...MockTask, status: "active" as const }];
		render(
			<TasksTable tasks={activeTasks} error={undefined} onRetry={() => {}} />,
		);

		await user.click(
			await screen.findByRole("button", { name: /pause task/i }),
		);

		await waitFor(() => {
			expect(stopWorkspaceSpy).toHaveBeenCalledWith(MockTask.workspace_id);
		});

		stopWorkspaceSpy.mockRestore();
	});

	it("calls startWorkspace on resume click", async () => {
		const user = userEvent.setup();
		const startWorkspaceSpy = vi
			.spyOn(API, "startWorkspace")
			.mockResolvedValue({} as never);

		const pausedTasks = [{ ...MockTask, status: "paused" as const }];
		render(
			<TasksTable tasks={pausedTasks} error={undefined} onRetry={() => {}} />,
		);

		await user.click(
			await screen.findByRole("button", { name: /resume task/i }),
		);

		await waitFor(() => {
			expect(startWorkspaceSpy).toHaveBeenCalledWith(
				MockTask.workspace_id,
				MockTask.template_version_id,
				undefined,
				undefined,
			);
		});

		startWorkspaceSpy.mockRestore();
	});

	it("renders loading state", async () => {
		render(
			<TasksTable tasks={undefined} error={undefined} onRetry={() => {}} />,
		);
		expect(await screen.findByRole("table")).toBeInTheDocument();
	});

	it("renders empty state", async () => {
		render(<TasksTable tasks={[]} error={undefined} onRetry={() => {}} />);
		expect(await screen.findByText(/no tasks found/i)).toBeInTheDocument();
	});

	it("renders error state", async () => {
		render(
			<TasksTable
				tasks={undefined}
				error={new Error("Failed to load tasks")}
				onRetry={() => {}}
			/>,
		);
		expect(
			await screen.findByText(/failed to load tasks/i),
		).toBeInTheDocument();
	});

	it("renders data state", async () => {
		render(
			<TasksTable tasks={MockTasks} error={undefined} onRetry={() => {}} />,
		);
		for (const task of MockTasks) {
			expect(await screen.findByText(task.display_name)).toBeInTheDocument();
		}
	});
});
