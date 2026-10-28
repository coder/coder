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

const mockGitHubMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-github",
	display_name: "GitHub",
	auth_type: "oauth2",
	auth_connected: false,
};

const mockNotionMCP: TypesGen.MCPServerConfig = {
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

// An iframe's window stands in for the sign-in popup that
// window.open returns.
const stubSignInPopup = () => {
	const iframe = document.createElement("iframe");
	document.body.appendChild(iframe);
	const popup = iframe.contentWindow;
	const open = vi.spyOn(window, "open").mockReturnValue(popup);
	return { popup, open, removePopup: () => iframe.remove() };
};

const completeSignIn = (popup: Window | null, serverID: string) => {
	act(() => {
		window.dispatchEvent(
			new MessageEvent("message", {
				data: { type: "mcp-oauth2-complete", serverID },
				origin: location.origin,
				source: popup,
			}),
		);
	});
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

	it("lists servers that need no sign-in after the OAuth2 servers", async () => {
		const mockRegistryMCP: TypesGen.MCPServerConfig = {
			...MockMCPServerConfig,
			id: "mcp-registry",
			display_name: "Registry",
			auth_type: "none",
		};
		setup(() => [mockRegistryMCP, mockGitHubMCP, mockNotionMCP]);
		renderPage();

		await screen.findByRole("button", { name: "Connect GitHub" });
		const names = screen
			.getAllByRole("row")
			.slice(1)
			.map((row) => within(row).getAllByRole("cell")[0]?.textContent);
		expect(names).toEqual(["GitHub", "Notion", "Registry"]);
	});

	it("connects through the organization's popup and refreshes the list when sign-in completes", async () => {
		const user = userEvent.setup();
		const getServers = setup(() => [mockGitHubMCP]);
		const { popup, open, removePopup } = stubSignInPopup();
		renderPage();

		await user.click(
			await screen.findByRole("button", { name: "Connect GitHub" }),
		);
		expect(open).toHaveBeenCalledWith(
			`/api/v2/organizations/${MockDefaultOrganization.id}/mcp-servers/${mockGitHubMCP.id}/oauth2/connect`,
			"_blank",
			"width=900,height=600",
		);

		getServers.mockResolvedValue([{ ...mockGitHubMCP, auth_connected: true }]);
		completeSignIn(popup, mockGitHubMCP.id);
		await waitFor(() => expect(getServers).toHaveBeenCalledTimes(2));
		removePopup();
	});

	it("confirms a sign-in that finishes after the organization changes", async () => {
		const user = userEvent.setup();
		setup((organizationId) =>
			organizationId === MockDefaultOrganization.id
				? [mockGitHubMCP]
				: [mockNotionMCP],
		);
		const { popup, removePopup } = stubSignInPopup();
		const toastSuccess = vi.spyOn(toast, "success");
		renderPage();

		await user.click(
			await screen.findByRole("button", { name: "Connect GitHub" }),
		);
		await user.click(
			screen.getByRole("combobox", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		);
		await user.click(
			screen.getByRole("option", {
				name: new RegExp(MockOrganization2.display_name),
			}),
		);
		await screen.findByRole("button", { name: "Disconnect Notion" });

		completeSignIn(popup, mockGitHubMCP.id);
		await waitFor(() =>
			expect(toastSuccess).toHaveBeenCalledWith("Connected GitHub."),
		);
		removePopup();
	});

	it("disconnects after confirmation", async () => {
		const user = userEvent.setup();
		setup(() => [mockNotionMCP]);
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
		expect(disconnect).toHaveBeenCalledWith(mockNotionMCP.id);
	});

	it("warns when the provider could not revoke the token", async () => {
		const user = userEvent.setup();
		setup(() => [mockNotionMCP]);
		vi.spyOn(API.experimental, "disconnectMCPServerOAuth2").mockResolvedValue({
			token_revoked: false,
			token_revocation_error: "The provider rejected the revocation.",
		});
		const toastWarning = vi.spyOn(toast, "warning");
		renderPage();

		await user.click(
			await screen.findByRole("button", { name: "Disconnect Notion" }),
		);
		await user.click(
			within(await screen.findByRole("dialog")).getByRole("button", {
				name: "Disconnect",
			}),
		);
		await waitFor(() =>
			expect(toastWarning).toHaveBeenCalledWith("Disconnected Notion.", {
				description: "The provider rejected the revocation.",
			}),
		);
	});

	it("reports a failed disconnect", async () => {
		const user = userEvent.setup();
		setup(() => [mockNotionMCP]);
		vi.spyOn(API.experimental, "disconnectMCPServerOAuth2").mockRejectedValue(
			new Error("disconnect failed"),
		);
		const toastError = vi.spyOn(toast, "error");
		renderPage();

		await user.click(
			await screen.findByRole("button", { name: "Disconnect Notion" }),
		);
		await user.click(
			within(await screen.findByRole("dialog")).getByRole("button", {
				name: "Disconnect",
			}),
		);
		await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));
	});

	it("does not disconnect when the confirmation is cancelled", async () => {
		const user = userEvent.setup();
		setup(() => [mockNotionMCP]);
		const disconnect = vi.spyOn(API.experimental, "disconnectMCPServerOAuth2");
		renderPage();

		await user.click(
			await screen.findByRole("button", { name: "Disconnect Notion" }),
		);
		await user.click(
			within(await screen.findByRole("dialog")).getByRole("button", {
				name: "Cancel",
			}),
		);
		expect(disconnect).not.toHaveBeenCalled();
	});

	it("starts with the chat's organization and lists another one on demand", async () => {
		const user = userEvent.setup();
		const getServers = setup((organizationId) =>
			organizationId === MockOrganization2.id
				? [mockGitHubMCP]
				: [mockNotionMCP],
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

	it("starts with the default organization when the chat's organization is not a member one", async () => {
		const getServers = setup(() => [mockGitHubMCP]);
		renderPage({ organizationId: "not-a-member-organization" });

		await screen.findByRole("table", {
			name: `MCP connections in ${MockDefaultOrganization.display_name}`,
		});
		expect(getServers).toHaveBeenCalledWith(MockDefaultOrganization.id);
	});
});
