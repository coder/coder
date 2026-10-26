import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { SESSION_TOKEN_PLACEHOLDER } from "#/modules/apps/apps";
import {
	MockBuildInfo,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import { stubLocationHref } from "#/testHelpers/location";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { WorkspacePill } from "./WorkspacePill";

const mockTokenApp = {
	...MockWorkspaceApp,
	external: true,
	display_name: "Token App",
	url: `vscode://coder.coder-remote/open?url=${new URL(MockBuildInfo.dashboard_url).origin}&token=${SESSION_TOKEN_PLACEHOLDER}`,
};

const renderWorkspacePill = () => {
	const getApiKey = vi
		.spyOn(API, "getApiKey")
		.mockResolvedValue({ key: "minted-key" });
	const navigate = stubLocationHref();
	renderWithAuth(
		<WorkspacePill
			workspace={MockWorkspace}
			agent={{ ...MockWorkspaceAgent, display_apps: [], apps: [mockTokenApp] }}
			chatId="chat-id"
		/>,
	);
	return { user: userEvent.setup(), getApiKey, navigate };
};

describe("WorkspacePill", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
		vi.restoreAllMocks();
	});

	it("does not mint another session key when a token app is activated again", async () => {
		const { user, getApiKey, navigate } = renderWorkspacePill();

		await user.click(
			await screen.findByRole("button", {
				name: `${MockWorkspace.name} workspace menu`,
			}),
		);
		(await screen.findByRole("menuitem", { name: "Token App" })).focus();
		await user.keyboard("{Enter}");
		await waitFor(() => expect(navigate).toHaveBeenCalledTimes(1));
		await user.keyboard("{Enter}");

		expect(getApiKey).toHaveBeenCalledTimes(1);
	});
});
