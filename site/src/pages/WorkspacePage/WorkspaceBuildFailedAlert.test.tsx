import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { takeDebugWorkspaceBuildIntent } from "#/modules/workspaces/workspaceBuildDebugLink";
import { MockFailedWorkspaceBuildWithUUID } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { isUUID } from "#/utils/uuid";
import { WorkspaceBuildFailedAlert } from "./WorkspaceBuildFailedAlert";

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("WorkspaceBuildFailedAlert", () => {
	it("records the click for the create page and reports it for telemetry", async () => {
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
		expect(
			takeDebugWorkspaceBuildIntent(MockFailedWorkspaceBuildWithUUID.id),
		).toBe(false);

		await user.click(link);

		expect(
			takeDebugWorkspaceBuildIntent(MockFailedWorkspaceBuildWithUUID.id),
		).toBe(true);
		await waitFor(() => expect(reportClick).toHaveBeenCalledTimes(1));
		const [buildId, request] = reportClick.mock.calls[0];
		expect(buildId).toBe(MockFailedWorkspaceBuildWithUUID.id);
		expect(isUUID(request.id)).toBe(true);
	});
});
