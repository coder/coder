import { screen } from "@testing-library/react";
import type { Task, Workspace, WorkspaceApp } from "#/api/typesGenerated";
import {
	MockTask,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { TaskApps } from "./TaskApps";

describe("TaskApps", () => {
	it("does not show apps marked as hidden", async () => {
		const visibleApp: WorkspaceApp = {
			...MockWorkspaceApp,
			id: "visible-app",
			slug: "visible-app",
			display_name: "Visible App",
			health: "healthy",
			hidden: false,
		};
		const hiddenApp: WorkspaceApp = {
			...MockWorkspaceApp,
			id: "hidden-app",
			slug: "hidden-app",
			display_name: "Hidden App",
			health: "healthy",
			hidden: true,
		};
		const task: Task = {
			...MockTask,
			workspace_app_id: null,
		};

		renderWithAuth(
			<TaskApps
				task={task}
				workspace={mockWorkspaceWithApps([visibleApp, hiddenApp])}
			/>,
		);

		expect(
			await screen.findByRole("link", { name: /visible app/i }),
		).toBeInTheDocument();
		expect(
			screen.queryByRole("link", { name: /hidden app/i }),
		).not.toBeInTheDocument();
	});
});

function mockWorkspaceWithApps(apps: WorkspaceApp[]): Workspace {
	return {
		...MockWorkspace,
		latest_build: {
			...MockWorkspace.latest_build,
			resources: [
				{
					...MockWorkspace.latest_build.resources[0],
					agents: [
						{
							...MockWorkspaceAgent,
							apps,
						},
					],
				},
			],
		},
	};
}
