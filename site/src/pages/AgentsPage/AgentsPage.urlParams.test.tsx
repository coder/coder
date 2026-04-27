import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat } from "#/api/typesGenerated";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import AgentsPage from "./AgentsPage";

vi.mock("#/components/Dialogs/ConfirmDialog/ConfirmDialog", () => ({
	ConfirmDialog: () => null,
}));

vi.mock("#/components/Dialogs/DeleteDialog/DeleteDialog", () => ({
	DeleteDialog: () => null,
}));

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		permissions: { editDeploymentConfig: false },
	}),
}));

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		appearance: { logo_url: "" },
	}),
}));

vi.mock("#/utils/reconnectingWebSocket", () => ({
	createReconnectingWebSocket: () => () => {},
}));

vi.mock("./hooks/useAgentsPageKeybindings", () => ({
	useAgentsPageKeybindings: () => {},
}));

vi.mock("./hooks/useAgentsPWA", () => ({
	useAgentsPWA: () => {},
}));

vi.mock("./AgentsPageView", () => ({
	AgentsPageView: ({
		archivedFilter,
		chatList,
		onArchivedFilterChange,
	}: {
		archivedFilter: "active" | "archived";
		chatList: readonly Chat[];
		onArchivedFilterChange: (filter: "active" | "archived") => void;
	}) => (
		<div>
			<div data-testid="archived-filter">{archivedFilter}</div>
			<button type="button" onClick={() => onArchivedFilterChange("active")}>
				Show active
			</button>
			<button type="button" onClick={() => onArchivedFilterChange("archived")}>
				Show archived
			</button>
			<ul>
				{chatList.map((chat) => (
					<li key={chat.id}>{chat.title}</li>
				))}
			</ul>
		</div>
	),
}));

const now = "2026-03-12T12:00:00.000Z";

const buildChat = (overrides: Partial<Chat>): Chat => ({
	id: "chat-default",
	organization_id: "test-org-id",
	owner_id: "owner-1",
	title: "Agent",
	status: "completed",
	last_model_config_id: "model-config-1",
	mcp_server_ids: [],
	labels: {},
	created_at: now,
	updated_at: now,
	archived: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	last_error: null,
	children: [],
	...overrides,
});

const activeChat = buildChat({
	id: "active-chat",
	title: "Active agent",
	archived: false,
});

const archivedChat = buildChat({
	id: "archived-chat",
	title: "Archived agent",
	archived: true,
});

const renderAgentsPage = (
	initialEntries: string[],
	initialIndex = initialEntries.length - 1,
) => {
	vi.spyOn(API.experimental, "getChats").mockImplementation(async (request) =>
		request?.q?.includes("archived:true") ? [archivedChat] : [activeChat],
	);
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		providers: [],
	});
	vi.spyOn(API.experimental, "getChatModelConfigs").mockResolvedValue([]);

	const queryClient = createTestQueryClient();
	const router = createMemoryRouter(
		[
			{ path: "/previous", element: <div>Previous page</div> },
			{ path: "/agents", element: <AgentsPage /> },
		],
		{ initialEntries, initialIndex },
	);

	render(
		<QueryClientProvider client={queryClient}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	);

	return router;
};

describe("AgentsPage archived URL filter", () => {
	it.each([
		{
			route: "/agents",
			expectedFilter: "active",
			expectedQuery: "archived:false",
			expectedTitle: activeChat.title,
		},
		{
			route: "/agents?archived=active",
			expectedFilter: "active",
			expectedQuery: "archived:false",
			expectedTitle: activeChat.title,
		},
		{
			route: "/agents?archived=archived",
			expectedFilter: "archived",
			expectedQuery: "archived:true",
			expectedTitle: archivedChat.title,
		},
		{
			route: "/agents?archived=garbage",
			expectedFilter: "active",
			expectedQuery: "archived:false",
			expectedTitle: activeChat.title,
		},
	])("loads $expectedFilter agents for $route", async ({
		route,
		expectedFilter,
		expectedQuery,
		expectedTitle,
	}) => {
		renderAgentsPage([route]);

		await waitFor(() => {
			expect(screen.getByTestId("archived-filter")).toHaveTextContent(
				expectedFilter,
			);
		});
		await screen.findByText(expectedTitle);
		expect(API.experimental.getChats).toHaveBeenCalledWith(
			expect.objectContaining({ q: expectedQuery }),
		);
	});

	it("replaces the current URL entry when toggling filters", async () => {
		const user = userEvent.setup();
		const router = renderAgentsPage(
			["/previous", "/agents?archived=active"],
			1,
		);

		await screen.findByText(activeChat.title);
		await user.click(screen.getByRole("button", { name: "Show archived" }));

		await waitFor(() => {
			expect(router.state.location.search).toBe("?archived=archived");
			expect(screen.getByTestId("archived-filter")).toHaveTextContent(
				"archived",
			);
		});
		await screen.findByText(archivedChat.title);

		await router.navigate(-1);

		await screen.findByText("Previous page");
	});

	it("restores the filter from the previous history URL", async () => {
		const router = renderAgentsPage(
			["/agents?archived=active", "/agents?archived=archived"],
			1,
		);

		await screen.findByText(archivedChat.title);
		expect(screen.getByTestId("archived-filter")).toHaveTextContent("archived");

		await router.navigate(-1);

		await waitFor(() => {
			expect(router.state.location.search).toBe("?archived=active");
			expect(screen.getByTestId("archived-filter")).toHaveTextContent("active");
		});
		await screen.findByText(activeChat.title);
	});
});
