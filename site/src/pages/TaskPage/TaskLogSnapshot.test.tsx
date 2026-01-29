import { MockTaskLogs } from "testHelpers/entities";
import { render } from "testHelpers/renderHelpers";
import { screen, waitFor } from "@testing-library/react";
import { API } from "api/api";
import { describe, expect, it, vi } from "vitest";
import { TaskLogSnapshot } from "./TaskLogSnapshot";

describe("TaskLogSnapshot", () => {
	it("shows loading state initially", async () => {
		vi.spyOn(API, "getTaskLogs").mockImplementation(
			() => new Promise(() => {}),
		);

		render(
			<TaskLogSnapshot
				username="testuser"
				taskId="test-task"
				actionLabel="Restart to view full logs"
			/>,
		);

		expect(await screen.findByTestId("loader")).toBeInTheDocument();
	});

	it("shows logs with user and agent prefixes", async () => {
		vi.spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogs);

		render(
			<TaskLogSnapshot
				username="testuser"
				taskId="test-task"
				actionLabel="Restart to view full logs"
			/>,
		);

		await waitFor(() => {
			expect(screen.getAllByText("[user]").length).toBeGreaterThan(0);
			expect(screen.getAllByText("[agent]").length).toBeGreaterThan(0);
		});

		expect(
			screen.getByText(/What's the latest GH issue\?/),
		).toBeInTheDocument();
		expect(screen.getByText(/I'll fetch that for you/)).toBeInTheDocument();
	});

	it("shows log count in header", async () => {
		vi.spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogs);

		render(
			<TaskLogSnapshot
				username="testuser"
				taskId="test-task"
				actionLabel="Restart to view full logs"
			/>,
		);

		await waitFor(() => {
			expect(
				screen.getByText(/Last 4 lines of AI chat logs/),
			).toBeInTheDocument();
		});
	});

	it("shows empty state when no logs", async () => {
		vi.spyOn(API, "getTaskLogs").mockResolvedValue({ logs: [] });

		render(
			<TaskLogSnapshot
				username="testuser"
				taskId="test-task"
				actionLabel="Restart to view full logs"
			/>,
		);

		await waitFor(() => {
			expect(
				screen.getByText(/No conversation history available/),
			).toBeInTheDocument();
		});
	});

	it("shows error state on fetch failure", async () => {
		vi.spyOn(API, "getTaskLogs").mockRejectedValue(new Error("Network error"));

		render(
			<TaskLogSnapshot
				username="testuser"
				taskId="test-task"
				actionLabel="Restart to view full logs"
			/>,
		);

		await waitFor(() => {
			expect(
				screen.getByText(/Unable to load conversation history/),
			).toBeInTheDocument();
		});
	});

	it("renders action label as link when actionHref provided", async () => {
		vi.spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogs);

		render(
			<TaskLogSnapshot
				username="testuser"
				taskId="test-task"
				actionLabel="View full logs"
				actionHref="/@testuser/workspace/builds/1"
			/>,
		);

		await waitFor(() => {
			const link = screen.getByRole("link", { name: /View full logs/ });
			expect(link).toBeInTheDocument();
			expect(link).toHaveAttribute("href", "/@testuser/workspace/builds/1");
		});
	});

	it("renders action label as text when no href or onAction", async () => {
		vi.spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogs);

		render(
			<TaskLogSnapshot
				username="testuser"
				taskId="test-task"
				actionLabel="Restart to view full logs"
			/>,
		);

		await waitFor(() => {
			expect(screen.getByText("Restart to view full logs")).toBeInTheDocument();
			expect(
				screen.queryByRole("link", { name: /Restart to view full logs/ }),
			).not.toBeInTheDocument();
		});
	});

	it("calls API with correct username and taskId", async () => {
		const getTaskLogsSpy = vi
			.spyOn(API, "getTaskLogs")
			.mockResolvedValue({ logs: [] });

		render(
			<TaskLogSnapshot
				username="myuser"
				taskId="my-task-123"
				actionLabel="Restart"
			/>,
		);

		await waitFor(() => {
			expect(getTaskLogsSpy).toHaveBeenCalledWith("myuser", "my-task-123");
		});

		getTaskLogsSpy.mockRestore();
	});
});
