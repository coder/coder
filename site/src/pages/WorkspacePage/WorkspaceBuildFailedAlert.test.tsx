import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { takeDebugWorkspaceBuildIntent } from "#/modules/workspaces/workspaceBuildDebugLink";
import { MockFailedWorkspaceBuildWithUUID } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { WorkspaceBuildFailedAlert } from "./WorkspaceBuildFailedAlert";

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("WorkspaceBuildFailedAlert", () => {
	it("records the click so the agents create page can send on the user's behalf", async () => {
		server.use(
			http.get("/api/v2/experiments", () =>
				HttpResponse.json(["enable-ai-workspace-debug"]),
			),
		);
		const user = userEvent.setup();

		renderWithAuth(
			<WorkspaceBuildFailedAlert build={MockFailedWorkspaceBuildWithUUID} />,
		);

		const link = await screen.findByRole("link", {
			name: "Debug with Coder Agents",
		});
		expect(link).toHaveAttribute(
			"href",
			`/agents?debug_workspace_build=${MockFailedWorkspaceBuildWithUUID.id}`,
		);
		expect(
			takeDebugWorkspaceBuildIntent(MockFailedWorkspaceBuildWithUUID.id),
		).toBe(false);

		await user.click(link);

		expect(
			takeDebugWorkspaceBuildIntent(MockFailedWorkspaceBuildWithUUID.id),
		).toBe(true);
	});
});
