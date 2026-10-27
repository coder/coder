import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Navigate } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockOrganization3,
	MockUserMember,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import AgentSettingsMCPConnectionsPage from "./AgentSettingsMCPConnectionsPage";

const githubMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-github",
	display_name: "GitHub",
	auth_type: "oauth2",
	auth_connected: false,
};

const notionMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-notion",
	display_name: "Notion",
	auth_type: "oauth2",
	auth_connected: true,
};

const setup = (
	getServers: (organizationId: string) => TypesGen.MCPServerConfig[],
) => {
	vi.spyOn(API, "getAuthenticatedUser").mockResolvedValue({
		...MockUserMember,
		organization_ids: [
			MockDefaultOrganization.id,
			MockOrganization2.id,
			MockOrganization3.id,
		],
	});
	vi.spyOn(API, "getOrganizations").mockResolvedValue([
		MockDefaultOrganization,
		MockOrganization2,
		MockOrganization3,
	]);
	return vi
		.spyOn(API.experimental, "getMCPServerConfigs")
		.mockImplementation(async (organizationId) => getServers(organizationId));
};

const renderPage = (state?: { organizationId: string }) =>
	renderWithAuth(<AgentSettingsMCPConnectionsPage />, {
		path: "/agents/settings/mcp-connections",
		route: "/start",
		extraRoutes: [
			{
				path: "/start",
				element: (
					<Navigate to="/agents/settings/mcp-connections" state={state} />
				),
			},
		],
	});

describe("AgentSettingsMCPConnectionsPage", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("connects through the organization's popup and refreshes the list when sign-in completes", async () => {
		const user = userEvent.setup();
		const getServers = setup(() => [githubMCP]);
		const iframe = document.createElement("iframe");
		document.body.appendChild(iframe);
		const popup = iframe.contentWindow;
		const open = vi.spyOn(window, "open").mockReturnValue(popup);
		renderPage();

		await user.click(
			await screen.findByRole("button", { name: "Connect GitHub" }),
		);
		expect(open).toHaveBeenCalledWith(
			`/api/v2/organizations/${MockDefaultOrganization.id}/mcp-servers/${githubMCP.id}/oauth2/connect`,
			"_blank",
			"width=900,height=600",
		);

		getServers.mockResolvedValue([{ ...githubMCP, auth_connected: true }]);
		act(() => {
			window.dispatchEvent(
				new MessageEvent("message", {
					data: { type: "mcp-oauth2-complete", serverID: githubMCP.id },
					origin: location.origin,
					source: popup,
				}),
			);
		});
		await waitFor(() => expect(getServers).toHaveBeenCalledTimes(2));
		iframe.remove();
	});

	it("disconnects after confirmation", async () => {
		const user = userEvent.setup();
		setup(() => [notionMCP]);
		const disconnect = vi
			.spyOn(API.experimental, "disconnectMCPServerOAuth2")
			.mockResolvedValue({ token_revoked: true });
		const toastSuccess = vi.spyOn(toast, "success");
		renderPage();

		await user.click(
			await screen.findByRole("button", { name: "Disconnect Notion" }),
		);
		const dialog = await screen.findByRole("dialog");
		await user.click(
			within(dialog).getByRole("button", { name: "Disconnect" }),
		);
		await waitFor(() =>
			expect(toastSuccess).toHaveBeenCalledWith("Disconnected Notion."),
		);
		expect(disconnect).toHaveBeenCalledWith(notionMCP.id);
	});

	it("starts with the chat's organization and lists another one on demand", async () => {
		const user = userEvent.setup();
		const getServers = setup((organizationId) =>
			organizationId === MockOrganization2.id ? [githubMCP] : [notionMCP],
		);
		renderPage({ organizationId: MockOrganization2.id });

		await screen.findByRole("table", {
			name: `MCP connections in ${MockOrganization2.display_name}`,
		});
		expect(getServers).toHaveBeenCalledWith(MockOrganization2.id);
		expect(getServers).not.toHaveBeenCalledWith(MockDefaultOrganization.id);

		await user.click(
			screen.getByRole("combobox", {
				name: `Organization ${MockOrganization2.display_name}`,
			}),
		);
		await user.click(
			screen.getByRole("option", {
				name: new RegExp(MockOrganization3.display_name),
			}),
		);
		await screen.findByRole("button", { name: "Disconnect Notion" });
		expect(getServers).toHaveBeenCalledWith(MockOrganization3.id);
	});
});
