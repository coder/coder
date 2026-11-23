import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { Link } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { chatAutomationsKey } from "#/api/queries/chatAutomations";
import { organizationChatModelsKey } from "#/api/queries/chats";
import type {
	Chat,
	CreateChatAutomationRequest,
	CreateChatAutomationResponse,
	OrganizationChatModelsResponse,
} from "#/api/typesGenerated";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockChat,
	MockChatAutomation,
	MockWebhookChatAutomation,
} from "#/testHelpers/chatEntities";
import {
	MockChatModelProviderDescriptor,
	MockDefaultChatModel,
	MockPersonalReasoningChatModel,
} from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockChatProject,
	MockDefaultOrganization,
	MockEntitlements,
	MockOrganization2,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreateAutomationPage from "./AgentCreateAutomationPage";

vi.mock("../components/AgentPageHeader", () => ({
	AgentPageHeader: () => <Link to="/agents/automations">Automations</Link>,
}));

const endpoint =
	"/api/experimental/organizations/:organizationId/chats/automations";
const modelsEndpoint = "/api/v2/organizations/:organizationId/chats/models";
const MockReasoningModel = {
	...MockPersonalReasoningChatModel,
	organization_id: MockDefaultOrganization.id,
};
const MockSecondModel = {
	...MockDefaultChatModel,
	id: "second-org-model",
	organization_id: MockOrganization2.id,
	display_name: "Second organization model",
};
const MockOwnedChat: Chat = {
	...MockChat,
	organization_id: MockDefaultOrganization.id,
	title: "Release preparation",
};
const MockSecondChat: Chat = {
	...MockOwnedChat,
	id: "second-org-chat",
	organization_id: MockOrganization2.id,
};
const mockModelCatalog: OrganizationChatModelsResponse = {
	models: [MockReasoningModel, MockDefaultChatModel],
	providers: [MockChatModelProviderDescriptor],
	unsupported_providers: [],
};
const mockScheduleResponse: CreateChatAutomationResponse = {
	automation: MockChatAutomation,
};

const renderPage = (enabled = true) =>
	renderWithAuth(<AgentCreateAutomationPage />, {
		path: "/agents/automations/new",
		route: "/agents/automations/new",
		extraRoutes: [
			{
				path: "/agents/automations",
				element: <Link to="/agents/automations/new">Create automation</Link>,
			},
			{ path: "/agents", element: <div /> },
		],
		mockAuthProviders: {
			DashboardProvider: ({ children }) => (
				<DashboardContext.Provider
					value={{
						entitlements: MockEntitlements,
						experiments: enabled ? ["chat-automations"] : [],
						appearance: MockAppearanceConfig,
						buildInfo: MockBuildInfo,
						organizations: [MockDefaultOrganization, MockOrganization2],
						showOrganizations: true,
						canViewOrganizationSettings: false,
					}}
				>
					{children}
				</DashboardContext.Provider>
			),
		},
	});

const select = async (
	user: ReturnType<typeof userEvent.setup>,
	label: string,
	option: string,
) => {
	await user.click(screen.getByLabelText(label));
	if (label === "Time zone") {
		await user.click(
			screen.getByRole("combobox", { name: "Search time zone" }),
		);
		await user.paste(option);
	}
	await user.click(await screen.findByRole("option", { name: option }));
};
const fillDetails = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(await screen.findByRole("textbox", { name: "Name" }));
	await user.paste("Daily report");
	await user.click(screen.getByRole("textbox", { name: "Prompt" }));
	await user.paste("Summarize changes.");
};
const chooseModel = (user: ReturnType<typeof userEvent.setup>) =>
	select(user, "Model", "GPT-5 (OpenAI)");
const submit = (user: ReturnType<typeof userEvent.setup>) =>
	user.click(screen.getByRole("button", { name: "Create automation" }));

beforeEach(() => {
	server.use(
		http.get(modelsEndpoint, ({ params }) =>
			HttpResponse.json<OrganizationChatModelsResponse>({
				...mockModelCatalog,
				models:
					params.organizationId === MockDefaultOrganization.id
						? mockModelCatalog.models
						: [MockSecondModel],
			}),
		),
		http.get("/api/experimental/chats/projects", () =>
			HttpResponse.json([MockChatProject]),
		),
		http.get("/api/v2/chats", () =>
			HttpResponse.json([MockOwnedChat, MockSecondChat]),
		),
	);
});
afterEach(() => vi.restoreAllMocks());

it("creates a fully configured schedule and invalidates only its organization's list", async () => {
	const user = userEvent.setup();
	const requests: {
		organizationId: string;
		req: CreateChatAutomationRequest;
	}[] = [];
	server.use(
		http.post<{ organizationId: string }, CreateChatAutomationRequest>(
			endpoint,
			async ({ params, request }) => {
				requests.push({
					organizationId: params.organizationId,
					req: await request.json(),
				});
				return HttpResponse.json(mockScheduleResponse, { status: 201 });
			},
		),
	);
	const { router, queryClient } = renderPage();
	for (const organization of [MockDefaultOrganization, MockOrganization2]) {
		queryClient.setQueryDefaults(chatAutomationsKey(organization.id), {
			gcTime: Number.POSITIVE_INFINITY,
		});
		queryClient.setQueryData(chatAutomationsKey(organization.id), []);
	}
	const name = "🛠".repeat(128);
	await user.click(await screen.findByRole("textbox", { name: "Name" }));
	await user.paste(` ${name} `);
	await user.click(screen.getByRole("textbox", { name: "Prompt" }));
	await user.paste("  Preserve prompt spacing.\n");
	await user.click(screen.getByRole("switch", { name: "Enabled" }));
	await user.clear(screen.getByRole("textbox", { name: "Cron schedule" }));
	await user.paste("30 8 * * 1");
	await select(user, "Time zone", "America/New_York");
	await chooseModel(user);
	await select(user, "Project", MockChatProject.name);
	await select(user, "Reasoning effort", "High");
	await submit(user);
	await waitFor(() =>
		expect(requests).toEqual([
			{
				organizationId: MockDefaultOrganization.id,
				req: {
					name,
					prompt: "  Preserve prompt spacing.\n",
					enabled: false,
					kind: "schedule",
					target_mode: "new_chat",
					schedule_cron: "30 8 * * 1",
					schedule_time_zone: "America/New_York",
					new_chat_model_config_id: MockReasoningModel.id,
					project_id: MockChatProject.id,
					reasoning_effort: "high",
				},
			},
		]),
	);
	await waitFor(() =>
		expect(
			queryClient.getQueryState(chatAutomationsKey(MockDefaultOrganization.id))
				?.isInvalidated,
		).toBe(true),
	);
	expect(
		queryClient.getQueryState(chatAutomationsKey(MockOrganization2.id))
			?.isInvalidated,
	).toBe(false);
	await user.click(
		await screen.findByRole("link", { name: "View automations" }),
	);
	await waitFor(() =>
		expect(router.state.location.pathname).toBe("/agents/automations"),
	);
});

it.each([
	{
		auth: "bearer",
		label: "Bearer token",
		target: "existing_chat",
		busy: "queue",
	},
	{
		auth: "github",
		label: "GitHub signature",
		target: "existing_chat",
		busy: "skip",
	},
	{
		auth: "standard_webhooks",
		label: "Standard Webhooks",
		target: "new_chat",
		busy: "queue",
	},
] as const)(
	"sends only applicable fields for $auth webhooks",
	async ({ auth, label, target, busy }) => {
		const user = userEvent.setup();
		const requests: CreateChatAutomationRequest[] = [];
		server.use(
			http.post<{ organizationId: string }, CreateChatAutomationRequest>(
				endpoint,
				async ({ request }) => {
					requests.push(await request.json());
					return HttpResponse.json(
						{ automation: MockWebhookChatAutomation },
						{ status: 201 },
					);
				},
			),
		);
		renderPage();
		await fillDetails(user);
		await chooseModel(user);
		await select(user, "Project", MockChatProject.name);
		await select(user, "Reasoning effort", "High");
		await select(user, "Target mode", "Use an existing chat");
		await select(user, "Existing chat", MockOwnedChat.title);
		await select(user, "Trigger type", "Webhook");
		await select(user, "Webhook authentication", "Standard Webhooks");
		await user.click(screen.getByLabelText("Webhook secret"));
		await user.paste("discarded-secret");
		if (auth === "standard_webhooks") {
			await select(user, "Webhook authentication", "GitHub signature");
		}
		await select(user, "Webhook authentication", label);
		if (target === "new_chat") {
			await select(user, "Target mode", "Start a new chat");
			await select(user, "Project", "None");
			await select(user, "Reasoning effort", "Model default");
			await submit(user);
			await waitFor(() =>
				expect(screen.getByLabelText("Webhook secret")).toHaveFocus(),
			);
			expect(requests).toEqual([]);
			await user.paste("whsec_c3Rvcnktc2VjcmV0");
		} else if (busy === "skip") {
			await select(user, "When the chat is busy", "Skip");
		}
		await submit(user);
		await waitFor(() =>
			expect(requests).toEqual([
				{
					name: "Daily report",
					prompt: "Summarize changes.",
					enabled: true,
					kind: "webhook",
					webhook_auth: auth,
					target_mode: target,
					...(target === "new_chat"
						? {
								new_chat_model_config_id: MockReasoningModel.id,
								webhook_secret: "whsec_c3Rvcnktc2VjcmV0",
							}
						: { target_chat_id: MockOwnedChat.id, when_busy: busy }),
				},
			]),
		);
	},
);

it("clears organization-scoped selections without discarding the automation draft", async () => {
	const user = userEvent.setup();
	const requests: {
		organizationId: string;
		req: CreateChatAutomationRequest;
	}[] = [];
	server.use(
		http.post<{ organizationId: string }, CreateChatAutomationRequest>(
			endpoint,
			async ({ params, request }) => {
				requests.push({
					organizationId: params.organizationId,
					req: await request.json(),
				});
				return HttpResponse.json(mockScheduleResponse, { status: 201 });
			},
		),
	);
	renderPage();
	await fillDetails(user);
	await chooseModel(user);
	await select(user, "Reasoning effort", "High");
	await select(user, "Project", MockChatProject.name);
	await select(user, "Target mode", "Use an existing chat");
	await select(user, "Existing chat", MockOwnedChat.title);
	await user.click(screen.getByRole("combobox", { name: "Organization *" }));
	await user.click(
		await screen.findByRole("option", {
			name: new RegExp(MockOrganization2.display_name),
		}),
	);
	await submit(user);
	await waitFor(() =>
		expect(screen.getByRole("button", { name: "Existing chat" })).toHaveFocus(),
	);
	expect(requests).toEqual([]);
	await select(user, "Target mode", "Start a new chat");
	await submit(user);
	await waitFor(() =>
		expect(screen.getByRole("button", { name: "Model" })).toHaveFocus(),
	);
	expect(requests).toEqual([]);
	await select(user, "Model", "Second organization model (OpenAI)");
	await submit(user);
	await waitFor(() =>
		expect(requests).toEqual([
			{
				organizationId: MockOrganization2.id,
				req: {
					name: "Daily report",
					prompt: "Summarize changes.",
					enabled: true,
					kind: "schedule",
					target_mode: "new_chat",
					schedule_cron: "0 9 * * 1-5",
					schedule_time_zone: "UTC",
					new_chat_model_config_id: MockSecondModel.id,
				},
			},
		]),
	);
});

it("retries a failed catalog and preserves edits through a background refetch", async () => {
	const user = userEvent.setup();
	let modelRequests = 0;
	const requests: CreateChatAutomationRequest[] = [];
	server.use(
		http.get(modelsEndpoint, () => {
			modelRequests++;
			return modelRequests === 1
				? HttpResponse.json({ message: "Models unavailable" }, { status: 503 })
				: HttpResponse.json(mockModelCatalog);
		}),
		http.post<{ organizationId: string }, CreateChatAutomationRequest>(
			endpoint,
			async ({ request }) => {
				requests.push(await request.json());
				return HttpResponse.json(mockScheduleResponse, { status: 201 });
			},
		),
	);
	const { queryClient } = renderPage();
	await fillDetails(user);
	await user.click(
		await screen.findByRole("button", { name: "Retry loading models" }),
	);
	await waitFor(() =>
		expect(
			queryClient.getQueryState(
				organizationChatModelsKey(MockDefaultOrganization.id),
			)?.status,
		).toBe("success"),
	);
	await chooseModel(user);
	await select(user, "Reasoning effort", "High");
	await select(user, "Model", "gpt-5 (OpenAI)");
	await act(() =>
		queryClient.invalidateQueries({
			queryKey: organizationChatModelsKey(MockDefaultOrganization.id),
		}),
	);
	await submit(user);
	await waitFor(() =>
		expect(requests).toEqual([
			{
				name: "Daily report",
				prompt: "Summarize changes.",
				enabled: true,
				kind: "schedule",
				target_mode: "new_chat",
				schedule_cron: "0 9 * * 1-5",
				schedule_time_zone: "UTC",
				new_chat_model_config_id: MockDefaultChatModel.id,
			},
		]),
	);
	expect(modelRequests).toBe(3);
});

it("focuses backend field errors and retries with the corrected draft", async () => {
	const user = userEvent.setup();
	const requests: CreateChatAutomationRequest[] = [];
	server.use(
		http.post<{ organizationId: string }, CreateChatAutomationRequest>(
			endpoint,
			async ({ request }) => {
				requests.push(await request.json());
				return requests.length === 1
					? HttpResponse.json(
							{
								message: "Invalid automation",
								validations: [
									{
										field: "schedule_cron",
										detail: "Runs must be at least one hour apart.",
									},
								],
							},
							{ status: 400 },
						)
					: HttpResponse.json(mockScheduleResponse, { status: 201 });
			},
		),
	);
	renderPage();
	await fillDetails(user);
	await chooseModel(user);
	await submit(user);
	await waitFor(() =>
		expect(
			screen.getByRole("textbox", { name: "Cron schedule" }),
		).toHaveFocus(),
	);
	await user.clear(screen.getByRole("textbox", { name: "Cron schedule" }));
	await user.paste("0 * * * *");
	await submit(user);
	await waitFor(() => expect(requests).toHaveLength(2));
	expect(requests[1]).toEqual({ ...requests[0], schedule_cron: "0 * * * *" });
});

it("prevents duplicate creation and navigation while a request is pending", async () => {
	const user = userEvent.setup();
	const response = createDeferred<undefined>();
	const requests: CreateChatAutomationRequest[] = [];
	server.use(
		http.post<{ organizationId: string }, CreateChatAutomationRequest>(
			endpoint,
			async ({ request }) => {
				requests.push(await request.json());
				await response.promise;
				return HttpResponse.json(mockScheduleResponse, { status: 201 });
			},
		),
	);
	try {
		const { router } = renderPage();
		await fillDetails(user);
		await chooseModel(user);
		await user.dblClick(
			screen.getByRole("button", { name: "Create automation" }),
		);
		await waitFor(() => expect(requests).toHaveLength(1));
		await user.click(screen.getByRole("button", { name: "Cancel" }));
		await user.click(screen.getByRole("link", { name: "Automations" }));
		const dialog = await screen.findByRole("dialog", {
			name: "Creating automation",
		});
		await user.click(within(dialog).getByRole("button", { name: "Leave" }));
		expect(router.state.location.pathname).toBe("/agents/automations/new");
		expect(requests).toHaveLength(1);
		await user.click(within(dialog).getByRole("button", { name: "Stay" }));
		response.resolve(undefined);
		await user.click(
			await screen.findByRole("link", { name: "View automations" }),
		);
		await waitFor(() =>
			expect(router.state.location.pathname).toBe("/agents/automations"),
		);
	} finally {
		response.resolve(undefined);
	}
});

it("warns before discarding a draft and restores focus when staying", async () => {
	const user = userEvent.setup();
	const { router } = renderPage();
	await fillDetails(user);
	await user.click(screen.getByRole("button", { name: "Cancel" }));
	const dialog = within(
		await screen.findByRole("dialog", { name: "Leave this page?" }),
	);
	await user.click(dialog.getByRole("button", { name: "Stay" }));
	expect(router.state.location.pathname).toBe("/agents/automations/new");
	await waitFor(() =>
		expect(screen.getByRole("button", { name: "Cancel" })).toHaveFocus(),
	);
	await user.click(screen.getByRole("button", { name: "Cancel" }));
	await user.click(
		within(
			await screen.findByRole("dialog", { name: "Leave this page?" }),
		).getByRole("button", { name: "Leave" }),
	);
	await waitFor(() =>
		expect(router.state.location.pathname).toBe("/agents/automations"),
	);
});

it("copies the one-time secret without retaining it in query caches and warns before leaving", async () => {
	const user = userEvent.setup();
	const copy = vi.spyOn(navigator.clipboard, "writeText");
	const secret = "test-only-one-time-webhook-secret";
	server.use(
		http.post(endpoint, () =>
			HttpResponse.json<CreateChatAutomationResponse>(
				{
					automation: MockWebhookChatAutomation,
					webhook_secret: secret,
				},
				{ status: 201 },
			),
		),
	);
	const { router, queryClient } = renderPage();
	await fillDetails(user);
	await chooseModel(user);
	await select(user, "Trigger type", "Webhook");
	await submit(user);
	await user.click(await screen.findByRole("button", { name: "Copy code" }));
	expect(copy).toHaveBeenCalledWith(secret);
	await waitFor(() =>
		expect(queryClient.getMutationCache().getAll()).toEqual([]),
	);
	expect(
		JSON.stringify(
			queryClient
				.getQueryCache()
				.getAll()
				.map((query) => query.state.data),
		),
	).not.toContain(secret);
	await user.click(screen.getByRole("link", { name: "View automations" }));
	await user.click(await screen.findByRole("button", { name: "Stay" }));
	expect(router.state.location.pathname).toBe("/agents/automations/new");
	await waitFor(() =>
		expect(
			screen.getByRole("link", { name: "View automations" }),
		).toHaveFocus(),
	);
	await user.click(
		screen.getByRole("checkbox", { name: "I have saved this secret" }),
	);
	await user.click(screen.getByRole("link", { name: "View automations" }));
	await waitFor(() =>
		expect(router.state.location.pathname).toBe("/agents/automations"),
	);
});

it("loads later chat pages and selects only an owned active root chat in the selected organization", async () => {
	const user = userEvent.setup();
	const chatRequests: URLSearchParams[] = [];
	const requests: CreateChatAutomationRequest[] = [];
	server.use(
		http.get("/api/v2/chats", ({ request }) => {
			const params = new URL(request.url).searchParams;
			chatRequests.push(params);
			const limit = Number(params.get("limit"));
			return HttpResponse.json(
				params.get("offset") === "0"
					? Array.from({ length: limit }, (_, index) => ({
							...MockSecondChat,
							id: `other-org-${index}`,
						}))
					: [
							{ ...MockOwnedChat, id: "other-owner", owner_id: "someone-else" },
							{ ...MockOwnedChat, id: "child", parent_chat_id: "parent" },
							{ ...MockOwnedChat, id: "archived", archived: true },
							MockOwnedChat,
						],
			);
		}),
		http.post<{ organizationId: string }, CreateChatAutomationRequest>(
			endpoint,
			async ({ request }) => {
				requests.push(await request.json());
				return HttpResponse.json(mockScheduleResponse, { status: 201 });
			},
		),
	);
	renderPage();
	await fillDetails(user);
	await select(user, "Target mode", "Use an existing chat");
	await waitFor(() => expect(chatRequests).toHaveLength(1));
	await user.click(screen.getByRole("button", { name: "Existing chat" }));
	await user.tab();
	expect(screen.getByRole("button", { name: "Load more chats" })).toHaveFocus();
	await user.keyboard("{Enter}");
	await user.click(
		await screen.findByRole("option", { name: MockOwnedChat.title }),
	);
	await submit(user);
	await waitFor(() =>
		expect(requests).toEqual([
			{
				name: "Daily report",
				prompt: "Summarize changes.",
				enabled: true,
				kind: "schedule",
				target_mode: "existing_chat",
				schedule_cron: "0 9 * * 1-5",
				schedule_time_zone: "UTC",
				target_chat_id: MockOwnedChat.id,
				when_busy: "skip",
			},
		]),
	);
	expect(chatRequests.map((params) => params.get("offset"))).toEqual([
		"0",
		"50",
	]);
	expect(chatRequests[0].get("q")).toBe("archived:false source:created_by_me");
});

it("redirects without loading resources when the experiment is off", async () => {
	const requested = vi.fn();
	server.use(
		http.get(modelsEndpoint, () => {
			requested();
			return HttpResponse.json(mockModelCatalog);
		}),
		http.get("/api/experimental/chats/projects", () => {
			requested();
			return HttpResponse.json([]);
		}),
		http.get("/api/v2/chats", () => {
			requested();
			return HttpResponse.json([]);
		}),
	);
	const { router } = renderPage(false);
	await waitFor(() => expect(router.state.location.pathname).toBe("/agents"));
	expect(requested).not.toHaveBeenCalled();
});
