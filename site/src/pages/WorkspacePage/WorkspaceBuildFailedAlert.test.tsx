import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { WorkspaceBuild } from "#/api/typesGenerated";
import {
	buildDebugWorkspaceBuildPath,
	takeDebugWorkspaceBuildIntent,
} from "#/modules/workspaces/workspaceBuildDebugLink";
import { MockFailedWorkspaceBuild } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { WorkspaceBuildFailedAlert } from "./WorkspaceBuildFailedAlert";

const failedBuild: WorkspaceBuild = {
	...MockFailedWorkspaceBuild("start"),
	id: "9f0e7d0e-4b2b-4ac9-8f1a-1a7a1f0c9d11",
};

const enableExperiment = () => {
	server.use(
		http.get("/api/v2/experiments", () =>
			HttpResponse.json(["enable-ai-workspace-debug"]),
		),
	);
};

const mockChatPermission = (createChatInOrganization: boolean) => {
	vi.spyOn(API, "checkAuthorization").mockImplementation(async (request) =>
		Object.fromEntries(
			Object.keys(request.checks).map((key) => [
				key,
				key === "createChatInOrganization" ? createChatInOrganization : true,
			]),
		),
	);
};

const renderAlert = () =>
	renderWithAuth(<WorkspaceBuildFailedAlert build={failedBuild} />);

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("WorkspaceBuildFailedAlert", () => {
	it("records the click and opens the agents create page for the build", async () => {
		enableExperiment();
		mockChatPermission(true);
		const user = userEvent.setup();

		renderAlert();

		const link = await screen.findByRole("link", {
			name: "Debug with Coder Agents",
		});
		expect(link).toHaveAttribute(
			"href",
			buildDebugWorkspaceBuildPath(failedBuild.id),
		);
		expect(link).toHaveAttribute("target", "_blank");
		expect(takeDebugWorkspaceBuildIntent(failedBuild.id)).toBe(false);

		await user.click(link);

		expect(takeDebugWorkspaceBuildIntent(failedBuild.id)).toBe(true);
	});

	it("is hidden without the experiment", async () => {
		mockChatPermission(true);

		renderAlert();

		await screen.findByText("Workspace build failed");
		expect(
			screen.queryByRole("link", { name: "Debug with Coder Agents" }),
		).not.toBeInTheDocument();
	});

	it("is hidden when the user cannot create chats in the build's organization", async () => {
		enableExperiment();
		mockChatPermission(false);

		renderAlert();

		await screen.findByText("Workspace build failed");
		await waitFor(() =>
			expect(API.checkAuthorization).toHaveBeenCalledWith(
				expect.objectContaining({
					checks: expect.objectContaining({
						createChatInOrganization: expect.objectContaining({
							object: expect.objectContaining({
								organization_id: failedBuild.job.organization_id,
							}),
						}),
					}),
				}),
			),
		);
		expect(
			screen.queryByRole("link", { name: "Debug with Coder Agents" }),
		).not.toBeInTheDocument();
	});
});
