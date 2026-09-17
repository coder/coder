import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	MockStoppedWorkspace,
	MockWorkspaceAgentOff,
	MockWorkspaceBuild,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { DesktopPanel } from "./DesktopPanel";

describe("DesktopPanel", () => {
	it("starts the workspace when it is stopped", async () => {
		const startWorkspace = vi
			.spyOn(API, "startWorkspace")
			.mockResolvedValue(MockWorkspaceBuild);

		render(
			<DesktopPanel
				chatId="chat-1"
				workspace={MockStoppedWorkspace}
				workspaceAgent={MockWorkspaceAgentOff}
				isVisible
			/>,
		);

		await userEvent.click(
			screen.getByRole("button", { name: /start workspace/i }),
		);

		await waitFor(() => {
			expect(startWorkspace).toHaveBeenCalledWith(
				MockStoppedWorkspace.id,
				MockStoppedWorkspace.latest_build.template_version_id,
				undefined,
				undefined,
			);
		});
	});
});
