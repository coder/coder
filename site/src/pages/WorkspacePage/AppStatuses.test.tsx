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
	MockWorkspaceAppStatus,
} from "#/testHelpers/entities";
import { stubLocationHref } from "#/testHelpers/location";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { AppStatuses } from "./AppStatuses";

const mockTokenApp = {
	...MockWorkspaceApp,
	external: true,
	url: `vscode://coder.coder-remote/open?url=${new URL(MockBuildInfo.dashboard_url).origin}&token=${SESSION_TOKEN_PLACEHOLDER}`,
	statuses: [{ ...MockWorkspaceAppStatus, uri: "" }],
};

const renderAppStatuses = () => {
	vi.spyOn(API, "getApiKey").mockResolvedValue({ key: "minted-key" });
	const navigate = stubLocationHref();
	renderWithAuth(
		<AppStatuses
			workspace={MockWorkspace}
			agent={{ ...MockWorkspaceAgent, apps: [mockTokenApp] }}
		/>,
	);
	return { user: userEvent.setup(), navigate };
};

describe("AppStatuses", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
		vi.restoreAllMocks();
	});

	it("opens a token app from the keyboard", async () => {
		const { user, navigate } = renderAppStatuses();

		await screen.findByRole("button", { name: MockWorkspaceApp.display_name });
		await user.tab();
		await user.keyboard("{Enter}");

		await waitFor(() =>
			expect(navigate).toHaveBeenCalledWith(
				mockTokenApp.url.replace(SESSION_TOKEN_PLACEHOLDER, "minted-key"),
			),
		);
	});
});
