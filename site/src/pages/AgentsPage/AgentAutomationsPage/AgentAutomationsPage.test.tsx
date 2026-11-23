import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { QueryClientProvider } from "react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { chatAutomationsKey } from "#/api/queries/chatAutomations";
import type { ChatAutomation } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockChatAutomation,
	MockWebhookChatAutomation,
} from "#/testHelpers/chatEntities";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockOrganization2,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import themes, { DEFAULT_THEME } from "#/theme";
import AgentAutomationsPage from "./AgentAutomationsPage";

vi.mock("../components/AgentPageHeader", () => ({
	AgentPageHeader: () => null,
}));

const organizations = [MockDefaultOrganization, MockOrganization2];
const mockSecondAutomation: ChatAutomation = {
	...MockWebhookChatAutomation,
	organization_id: MockOrganization2.id,
};
const endpoint =
	"/api/experimental/organizations/:organizationId/chats/automations";

const renderPage = (enabled = true) => {
	const queryClient = createTestQueryClient();
	const router = createMemoryRouter(
		[
			{ path: "/agents/automations", element: <AgentAutomationsPage /> },
			{ path: "/agents/automations/new", element: <div /> },
			{ path: "/agents/:chatId", element: <div /> },
			{ path: "/agents", element: <div /> },
		],
		{ initialEntries: ["/agents/automations"] },
	);
	render(
		<QueryClientProvider client={queryClient}>
			<ThemeOverride theme={themes[DEFAULT_THEME]}>
				<TooltipProvider>
					<DashboardContext.Provider
						value={{
							entitlements: MockEntitlements,
							experiments: enabled ? ["chat-automations"] : [],
							appearance: MockAppearanceConfig,
							buildInfo: MockBuildInfo,
							organizations,
							showOrganizations: false,
							canViewOrganizationSettings: false,
						}}
					>
						<RouterProvider router={router} />
					</DashboardContext.Provider>
				</TooltipProvider>
			</ThemeOverride>
		</QueryClientProvider>,
	);
	return { router, queryClient };
};

const openDelete = async (
	user: ReturnType<typeof userEvent.setup>,
	automation: ChatAutomation,
) => {
	await user.click(
		screen.getByRole("button", { name: `More actions for ${automation.name}` }),
	);
	await user.click(screen.getByRole("menuitem", { name: "Delete…" }));
	await screen.findByRole("dialog", { name: "Delete automation" });
};

afterEach(() => vi.restoreAllMocks());

it("fetches both organizations, cancels safely, then deletes in the right organization and refreshes only its list", async () => {
	const user = userEvent.setup();
	const fetched: string[] = [];
	const deleted: string[] = [];
	server.use(
		http.get(endpoint, ({ params }) => {
			const orgId = String(params.organizationId);
			fetched.push(orgId);
			if (orgId === MockOrganization2.id) {
				return HttpResponse.json(
					deleted.length === 0 ? [mockSecondAutomation] : [],
				);
			}
			return HttpResponse.json([MockChatAutomation]);
		}),
		http.delete(`${endpoint}/:automationId`, ({ params }) => {
			deleted.push(`${params.organizationId}/${params.automationId}`);
			return new HttpResponse(null, { status: 204 });
		}),
	);
	const { queryClient } = renderPage();
	await waitFor(() =>
		expect(fetched).toEqual(
			expect.arrayContaining(organizations.map((org) => org.id)),
		),
	);
	await waitFor(() =>
		expect(
			queryClient.getQueryData(chatAutomationsKey(MockOrganization2.id)),
		).toEqual([mockSecondAutomation]),
	);
	await openDelete(user, mockSecondAutomation);
	await user.click(screen.getByRole("button", { name: "Cancel" }));
	expect(deleted).toEqual([]);
	await waitFor(() =>
		expect(
			screen.getByRole("button", {
				name: `More actions for ${mockSecondAutomation.name}`,
			}),
		).toHaveFocus(),
	);
	await user.keyboard("{Enter}");
	await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
	await user.type(
		await screen.findByRole("textbox", {
			name: "Name of the automation to delete",
		}),
		mockSecondAutomation.name,
	);
	await user.click(screen.getByRole("button", { name: "Delete" }));
	await waitFor(() =>
		expect(deleted).toEqual([
			`${MockOrganization2.id}/${mockSecondAutomation.id}`,
		]),
	);
	await waitFor(() =>
		expect(fetched.filter((id) => id === MockOrganization2.id)).toHaveLength(2),
	);
	expect(
		fetched.filter((id) => id === MockDefaultOrganization.id),
	).toHaveLength(1);
	await waitFor(() =>
		expect(screen.getByRole("region", { name: "Automations" })).toHaveFocus(),
	);
});

it("keeps the failed deletion actionable and reports the server error", async () => {
	const user = userEvent.setup();
	const deleted: string[] = [];
	const errorToast = vi.spyOn(toast, "error").mockImplementation(() => "error");
	server.use(
		http.get(endpoint, ({ params }) =>
			HttpResponse.json(
				params.organizationId === MockDefaultOrganization.id
					? [MockChatAutomation]
					: [],
			),
		),
		http.delete(`${endpoint}/:automationId`, ({ params }) => {
			deleted.push(String(params.automationId));
			return HttpResponse.json({ message: "Deletion denied" }, { status: 403 });
		}),
	);
	renderPage();
	await screen.findByRole("button", {
		name: `More actions for ${MockChatAutomation.name}`,
	});
	await openDelete(user, MockChatAutomation);
	await user.type(
		screen.getByRole("textbox", { name: "Name of the automation to delete" }),
		MockChatAutomation.name,
	);
	await user.click(screen.getByRole("button", { name: "Delete" }));
	await waitFor(() =>
		expect(errorToast).toHaveBeenCalledWith("Deletion denied"),
	);
	await user.click(screen.getByRole("button", { name: "Delete" }));
	await waitFor(() => expect(deleted).toHaveLength(2));
});

it("retries only the failed organization while keeping the filter and usable rows", async () => {
	const user = userEvent.setup();
	const requests: string[] = [];
	server.use(
		http.get(endpoint, ({ params }) => {
			const id = String(params.organizationId);
			requests.push(id);
			if (
				id === MockOrganization2.id &&
				requests.filter((value) => value === id).length === 1
			) {
				return HttpResponse.json({ message: "Unavailable" }, { status: 503 });
			}
			return HttpResponse.json(
				id === MockOrganization2.id
					? [mockSecondAutomation]
					: [MockChatAutomation],
			);
		}),
	);
	const { queryClient } = renderPage();
	await screen.findByRole("button", {
		name: `Retry loading automations for ${MockOrganization2.display_name}`,
	});
	await user.type(
		screen.getByRole("combobox", { name: /Search and filter automations/ }),
		"Daily PR",
	);
	await user.click(
		screen.getByRole("button", {
			name: `More actions for ${MockChatAutomation.name}`,
		}),
	);
	await user.click(screen.getByRole("menuitem", { name: "Delete…" }));
	await user.click(await screen.findByRole("button", { name: "Cancel" }));
	await user.click(
		await screen.findByRole("button", {
			name: `Retry loading automations for ${MockOrganization2.display_name}`,
		}),
	);
	await waitFor(() =>
		expect(
			queryClient.getQueryData(chatAutomationsKey(MockOrganization2.id)),
		).toEqual([mockSecondAutomation]),
	);
	expect(
		requests.filter((id) => id === MockDefaultOrganization.id),
	).toHaveLength(1);
	expect(requests.filter((id) => id === MockOrganization2.id)).toHaveLength(2);
	await user.click(screen.getByRole("button", { name: /More actions for/ }));
	await user.click(screen.getByRole("menuitem", { name: "Delete…" }));
	await user.click(await screen.findByRole("button", { name: "Cancel" }));
});

it("navigates to a readable existing target chat and to the creation page", async () => {
	const user = userEvent.setup();
	server.use(
		http.get(endpoint, ({ params }) =>
			HttpResponse.json(
				params.organizationId === MockDefaultOrganization.id
					? [MockChatAutomation]
					: [],
			),
		),
	);
	const { router } = renderPage();
	await user.click(
		await screen.findByRole("button", {
			name: `More actions for ${MockChatAutomation.name}`,
		}),
	);
	await user.click(screen.getByRole("menuitem", { name: "View chat" }));
	await waitFor(() =>
		expect(router.state.location.pathname).toBe(
			`/agents/${MockChatAutomation.target_chat_id}`,
		),
	);
	await act(() => router.navigate("/agents/automations"));
	await user.click(
		await screen.findByRole("link", { name: "Create automation" }),
	);
	await waitFor(() =>
		expect(router.state.location.pathname).toBe("/agents/automations/new"),
	);
});

it("redirects without requesting lists when the experiment is off", async () => {
	const requests: string[] = [];
	server.use(
		http.get(endpoint, ({ params }) => {
			requests.push(String(params.organizationId));
			return HttpResponse.json([]);
		}),
	);
	const { router } = renderPage(false);
	await waitFor(() => expect(router.state.location.pathname).toBe("/agents"));
	expect(requests).toEqual([]);
});
