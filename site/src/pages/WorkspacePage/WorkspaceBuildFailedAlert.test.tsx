import { screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { MockFailedWorkspaceBuild } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { WorkspaceBuildFailedAlert } from "./WorkspaceBuildFailedAlert";

const failedBuild = MockFailedWorkspaceBuild();

describe("WorkspaceBuildFailedAlert", () => {
	it("links to the agents page for the failed build in a new tab", async () => {
		server.use(
			http.get("/api/v2/experiments", () =>
				HttpResponse.json(["enable-ai-workspace-debug"]),
			),
		);

		renderWithAuth(<WorkspaceBuildFailedAlert build={failedBuild} />);

		const link = await screen.findByRole("link", {
			name: "Debug with Coder Agents",
		});
		expect(link).toHaveAttribute(
			"href",
			`/agents?debug_workspace_build=${failedBuild.id}`,
		);
		expect(link).toHaveAttribute("target", "_blank");
	});
});
