import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockFailedWorkspaceBuildWithUUID } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { isUUID } from "#/utils/uuid";
import { WorkspaceBuildFailedAlert } from "./WorkspaceBuildFailedAlert";

afterEach(() => {
	vi.restoreAllMocks();
});

describe("WorkspaceBuildFailedAlert", () => {
	it("links to the agents page in a new tab and reports the click", async () => {
		server.use(
			http.get("/api/v2/experiments", () =>
				HttpResponse.json(["enable-ai-workspace-debug"]),
			),
		);
		const reportClick = vi
			.spyOn(API, "reportWorkspaceBuildDebugClick")
			.mockResolvedValue();
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
		expect(link).toHaveAttribute("target", "_blank");

		await user.click(link);

		await waitFor(() => expect(reportClick).toHaveBeenCalledTimes(1));
		const [buildId, request] = reportClick.mock.calls[0];
		expect(buildId).toBe(MockFailedWorkspaceBuildWithUUID.id);
		expect(isUUID(request.id)).toBe(true);
	});
});
