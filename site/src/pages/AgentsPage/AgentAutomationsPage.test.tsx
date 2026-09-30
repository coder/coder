import { screen, waitFor, within } from "@testing-library/react";
import userEvent, {
	PointerEventsCheckLevel,
} from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Chat, ChatAutomation, ChatModel } from "#/api/typesGenerated";
import {
	MockChat,
	MockChatAutomation,
	MockWebhookChatAutomation,
} from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentAutomationsPage from "./AgentAutomationsPage";
import { selectedOrganizationIdStorageKey } from "./components/AgentCreateForm";

// AgentPageHeader needs the layout's outlet context.
vi.mock("./components/AgentPageHeader", () => ({
	AgentPageHeader: () => null,
}));

const mockAutomation: ChatAutomation = {
	...MockChatAutomation,
	organization_id: MockDefaultOrganization.id,
	target_chat_id: MockChat.id,
};

const mockWebhookAutomation: ChatAutomation = {
	...MockWebhookChatAutomation,
	organization_id: MockDefaultOrganization.id,
	target_chat_id: MockChat.id,
};

const automationsPath = (organizationId: string) =>
	`/api/experimental/organizations/${organizationId}/chat-automations`;

const setup = ({
	experiments = ["chat-automations"],
	automations = [mockAutomation],
} = {}) => {
	const requests: Request[] = [];
	server.use(
		http.get("/api/v2/experiments", () => HttpResponse.json(experiments)),
		http.get("/api/v2/organizations", () =>
			HttpResponse.json([MockDefaultOrganization, MockOrganization2]),
		),
		http.get("/api/v2/chats/:chatId", () => HttpResponse.json(MockChat)),
		http.get("/api/v2/chats", ({ request }) => {
			requests.push(request);
			return HttpResponse.json([MockChat]);
		}),
		http.all(
			"/api/experimental/organizations/:organizationId/chat-automations*",
			({ request }) => {
				requests.push(request);
				return undefined;
			},
		),
		http.get(automationsPath(":organizationId"), () =>
			HttpResponse.json(automations),
		),
	);
	const { queryClient } = renderWithAuth(<AgentAutomationsPage />);
	return { requests, queryClient };
};

const requestPaths = (requests: readonly Request[]) =>
	requests.map((request) => {
		const url = new URL(request.url);
		return `${request.method} ${url.pathname}${url.search}`;
	});

afterEach(() => {
	localStorage.clear();
});

describe("AgentAutomationsPage", () => {
	it("lists the automations of the organization the Agents picker selected", async () => {
		const user = userEvent.setup();
		localStorage.setItem(
			selectedOrganizationIdStorageKey,
			MockOrganization2.id,
		);
		const { requests } = setup();

		await waitFor(() => {
			expect(requestPaths(requests)).toContain(
				`GET ${automationsPath(MockOrganization2.id)}`,
			);
		});
		expect(requestPaths(requests)).not.toContain(
			`GET ${automationsPath(MockDefaultOrganization.id)}`,
		);

		await user.click(
			await screen.findByRole("button", {
				name: `Organization: ${MockOrganization2.display_name}`,
			}),
		);
		await user.click(
			await screen.findByRole("option", {
				name: MockDefaultOrganization.display_name,
			}),
		);
		await waitFor(() => {
			expect(requestPaths(requests)).toContain(
				`GET ${automationsPath(MockDefaultOrganization.id)}`,
			);
		});
		expect(localStorage.getItem(selectedOrganizationIdStorageKey)).toBe(
			MockDefaultOrganization.id,
		);
	});

	it("does not request automations when the experiment is off", async () => {
		const { requests } = setup({ experiments: [] });

		await screen.findByText("This page could not be found.");
		expect(requests).toEqual([]);
	});

	it("disables an automation with the enabled switch", async () => {
		const user = userEvent.setup();
		let body: unknown;
		setup();
		server.use(
			http.patch(
				`${automationsPath(MockDefaultOrganization.id)}/${mockAutomation.id}`,
				async ({ request }) => {
					body = await request.json();
					return HttpResponse.json({ ...mockAutomation, enabled: false });
				},
			),
		);

		await user.click(
			await screen.findByRole("switch", {
				name: `Enable ${mockAutomation.name}`,
			}),
		);

		await waitFor(() => {
			expect(body).toEqual({ enabled: false });
		});
	});

	it.each([
		[409, "The target chat is busy."],
		[429, "The automation used up its share of the chat queue."],
	])(
		"sends the Run now request when the server answers %i",
		async (status, message) => {
			const user = userEvent.setup();
			const { requests } = setup();
			const runPath = `${automationsPath(MockDefaultOrganization.id)}/${mockAutomation.id}/runs`;
			server.use(
				http.post(runPath, ({ request }) => {
					requests.push(request);
					return HttpResponse.json({ message }, { status });
				}),
			);

			await user.click(
				await screen.findByRole("button", {
					name: `Run now ${mockAutomation.name}`,
				}),
			);

			await waitFor(() => {
				expect(requestPaths(requests)).toContain(`POST ${runPath}`);
			});
		},
	);

	it("lists an automation's chats with the automation filter", async () => {
		const user = userEvent.setup();
		const { requests } = setup();

		await user.click(
			await screen.findByRole("button", {
				name: `View chats ${mockAutomation.name}`,
			}),
		);

		await waitFor(() => {
			expect(requestPaths(requests)).toContain(
				`GET /api/v2/chats?automation_id=${mockAutomation.id}&limit=25&offset=0`,
			);
		});
	});

	it("loads more of an automation's chats", async () => {
		const user = userEvent.setup();
		const { requests } = setup();
		const mockChatPage = Array.from({ length: 25 }, (_, index) => ({
			...MockChat,
			id: `chat-${index}`,
		}));
		server.use(
			http.get("/api/v2/chats", ({ request }) => {
				requests.push(request);
				return HttpResponse.json(
					new URL(request.url).searchParams.get("offset") === "0"
						? mockChatPage
						: [MockChat],
				);
			}),
		);

		await user.click(
			await screen.findByRole("button", {
				name: `View chats ${mockAutomation.name}`,
			}),
		);
		await user.click(await screen.findByRole("button", { name: "Load more" }));

		await waitFor(() => {
			expect(requestPaths(requests)).toContain(
				`GET /api/v2/chats?automation_id=${mockAutomation.id}&limit=25&offset=25`,
			);
		});
	});
});

const mockTargetChat: Chat = {
	...MockChat,
	organization_id: MockDefaultOrganization.id,
};

const mockOtherChat: Chat = {
	...mockTargetChat,
	id: "chat-2",
	title: "Release notes",
};

const mockOtherOrgChat: Chat = {
	...MockChat,
	id: "chat-3",
	organization_id: MockOrganization2.id,
	title: "Other organization chat",
};

const mockModel: ChatModel = {
	...MockChatModel,
	organization_id: MockDefaultOrganization.id,
	reasoning_efforts: ["low", "high"],
};

const browserTimeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;

const validationError = (field: string, detail: string) =>
	HttpResponse.json(
		{ message: "Invalid chat automation.", validations: [{ field, detail }] },
		{ status: 400 },
	);

const setupEditor = (options?: Parameters<typeof setup>[0]) => {
	const previewBodies: unknown[] = [];
	const createBodies: unknown[] = [];
	const updateBodies: unknown[] = [];
	const { requests, queryClient } = setup(options);
	// Every published mutation state, as a cache subscriber like devtools sees it.
	const mutationStates: string[] = [];
	queryClient.getMutationCache().subscribe(({ mutation }) => {
		mutationStates.push(JSON.stringify(mutation?.state));
	});
	server.use(
		http.get("/api/v2/chats", ({ request }) => {
			requests.push(request);
			return HttpResponse.json([
				mockTargetChat,
				mockOtherChat,
				mockOtherOrgChat,
			]);
		}),
		http.get("/api/v2/organizations/:organizationId/chats/models", () =>
			HttpResponse.json({
				models: [mockModel],
				providers: [MockChatModelProviderDescriptor],
				unsupported_providers: [],
			}),
		),
		http.post(
			`${automationsPath(":organizationId")}/schedule-preview`,
			async ({ request }) => {
				const body = await request.json();
				previewBodies.push(body);
				return HttpResponse.json({
					next_run_times: ["2026-10-01T09:30:00Z"],
				});
			},
		),
		http.post(automationsPath(":organizationId"), async ({ request }) => {
			createBodies.push(await request.json());
			return HttpResponse.json({ automation: mockAutomation }, { status: 201 });
		}),
		http.patch(
			`${automationsPath(":organizationId")}/:automationId`,
			async ({ request }) => {
				updateBodies.push(await request.json());
				return HttpResponse.json(mockAutomation);
			},
		),
	);
	return {
		requests,
		queryClient,
		mutationStates,
		previewBodies,
		createBodies,
		updateBodies,
	};
};

const openCreateDialog = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(
		await screen.findByRole("button", { name: "New automation" }),
	);
	const dialog = await screen.findByRole("dialog");
	await user.click(within(dialog).getByLabelText(/^Name/));
	await user.paste("Standup");
	await user.click(within(dialog).getByLabelText(/^Prompt/));
	await user.paste("Summarize yesterday.");
	return dialog;
};

const pickChat = async (
	user: ReturnType<typeof userEvent.setup>,
	dialog: HTMLElement,
	title: string,
) => {
	await user.click(within(dialog).getByRole("button", { name: "Chat" }));
	await user.click(await screen.findByRole("option", { name: title }));
};

const pickNewChatModel = async (
	user: ReturnType<typeof userEvent.setup>,
	dialog: HTMLElement,
) => {
	await user.click(
		within(dialog).getByRole("radio", { name: "New chat each run" }),
	);
	await user.click(
		await within(dialog).findByRole("combobox", { name: /^Model/ }),
	);
	await user.click(
		await screen.findByRole("option", {
			name: new RegExp(mockModel.display_name),
		}),
	);
};

// Each editor test drives several Radix popovers, which is slow in jsdom.
describe("AgentAutomationsPage editor", { timeout: 15_000 }, () => {
	it("creates an existing chat schedule from the Repeat and Time shortcuts", async () => {
		const user = userEvent.setup();
		const { requests, previewBodies, createBodies } = setupEditor();
		const dialog = await openCreateDialog(user);

		await user.click(within(dialog).getByRole("combobox", { name: "Repeat" }));
		await user.click(await screen.findByRole("option", { name: "Weekdays" }));
		const time = within(dialog).getByLabelText("Time");
		await user.clear(time);
		await user.type(time, "09:30");
		expect(within(dialog).getByLabelText(/^Cron expression/)).toHaveValue(
			"30 9 * * 1-5",
		);
		await waitFor(() => {
			expect(previewBodies).toContainEqual({
				schedule_cron: "30 9 * * 1-5",
				schedule_time_zone: browserTimeZone,
			});
		});

		await user.click(within(dialog).getByRole("button", { name: "Chat" }));
		await user.type(await screen.findByPlaceholderText("Search chats"), "Rele");
		await waitFor(() => {
			expect(
				requests.map((request) => new URL(request.url).searchParams.get("q")),
			).toContain('title:"Rele" archived:false source:created_by_me');
		});
		await user.click(
			await screen.findByRole("option", { name: mockOtherChat.title }),
		);
		await user.click(within(dialog).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(createBodies).toEqual([
				{
					name: "Standup",
					kind: "schedule",
					target_mode: "existing_chat",
					prompt: "Summarize yesterday.",
					schedule_cron: "30 9 * * 1-5",
					schedule_time_zone: browserTimeZone,
					target_chat_id: mockOtherChat.id,
					when_busy: "skip",
				},
			]);
		});
	});

	it.each([
		{ source: "the model default", keys: undefined, effort: {} },
		{
			source: "a picked effort",
			keys: "{ArrowLeft}",
			effort: { reasoning_effort: "low" },
		},
	])("creates a new chat schedule with $source", async ({ keys, effort }) => {
		const user = userEvent.setup();
		const { createBodies } = setupEditor();
		const dialog = await openCreateDialog(user);

		await pickNewChatModel(user, dialog);
		if (keys) {
			(await screen.findByRole("slider")).focus();
			await user.keyboard(keys);
		}
		await user.keyboard("{Escape}");
		await user.click(within(dialog).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(createBodies).toEqual([
				{
					name: "Standup",
					kind: "schedule",
					target_mode: "new_chat",
					prompt: "Summarize yesterday.",
					schedule_cron: "0 9 * * *",
					schedule_time_zone: browserTimeZone,
					new_chat_model_config_id: mockModel.id,
					...effort,
				},
			]);
		});
	});

	it("shows every required field error on the first Save", async () => {
		const user = userEvent.setup();
		const { createBodies } = setupEditor();
		await user.click(
			await screen.findByRole("button", { name: "New automation" }),
		);
		const dialog = await screen.findByRole("dialog");

		await user.click(within(dialog).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(
				within(dialog).getByLabelText(/^Name/),
			).toHaveAccessibleDescription("Name is required.");
		});
		expect(
			within(dialog).getByLabelText(/^Prompt/),
		).toHaveAccessibleDescription("Prompt is required.");
		expect(
			within(dialog).getByRole("button", { name: "Chat" }),
		).toHaveAccessibleDescription("Choose a chat.");

		await user.click(
			within(dialog).getByRole("radio", { name: "New chat each run" }),
		);
		expect(
			await within(dialog).findByRole("combobox", { name: /^Model/ }),
		).toHaveAccessibleDescription("Choose a model.");
		expect(createBodies).toEqual([]);
	});

	it("shows the server's cron validation errors on the cron input", async () => {
		const user = userEvent.setup();
		setupEditor();
		server.use(
			http.post(`${automationsPath(":organizationId")}/schedule-preview`, () =>
				validationError("schedule_cron", "Expected exactly five fields."),
			),
			http.post(automationsPath(":organizationId"), () =>
				validationError("schedule_cron", "Must be a valid cron expression."),
			),
		);
		const dialog = await openCreateDialog(user);
		const cron = within(dialog).getByLabelText(/^Cron expression/);

		await user.clear(cron);
		await user.type(cron, "bad");
		await waitFor(() => {
			expect(cron).toHaveAccessibleDescription(
				expect.stringContaining("Expected exactly five fields."),
			);
		});
		expect(
			within(dialog).getAllByText("Expected exactly five fields."),
		).toHaveLength(1);

		await pickChat(user, dialog, MockChat.title);
		await user.click(within(dialog).getByRole("button", { name: "Save" }));
		await waitFor(() => {
			expect(cron).toHaveAccessibleDescription(
				expect.stringContaining("Must be a valid cron expression."),
			);
		});
		expect(within(dialog).queryByRole("alert")).toBeNull();

		await user.type(cron, "x");
		expect(cron).toHaveAccessibleDescription(
			expect.not.stringContaining("Must be a valid cron expression."),
		);
		await user.click(within(dialog).getByRole("radio", { name: "Webhook" }));
		expect(within(dialog).queryByRole("alert")).toBeNull();
	});

	it("offers only chats from the automation's organization", async () => {
		const user = userEvent.setup();
		setupEditor();
		const dialog = await openCreateDialog(user);

		await user.click(within(dialog).getByRole("button", { name: "Chat" }));
		await screen.findByRole("option", { name: mockOtherChat.title });
		expect(
			screen.getAllByRole("option").map((option) => option.textContent),
		).toEqual([mockTargetChat.title, mockOtherChat.title]);
	});

	it("saves a name of 128 code points that spans more UTF-16 units", async () => {
		const user = userEvent.setup();
		const { createBodies } = setupEditor();
		await user.click(
			await screen.findByRole("button", { name: "New automation" }),
		);
		const dialog = await screen.findByRole("dialog");
		const name = "\u{1F600}".repeat(65) + "x".repeat(63);
		await user.click(within(dialog).getByLabelText(/^Name/));
		await user.paste(name);
		await user.click(within(dialog).getByLabelText(/^Prompt/));
		await user.paste("Summarize yesterday.");
		await pickChat(user, dialog, mockTargetChat.title);
		await user.click(within(dialog).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(createBodies).toEqual([expect.objectContaining({ name })]);
		});
	});

	it("shows a time zone preview error on the time zone field", async () => {
		const user = userEvent.setup();
		setupEditor();
		server.use(
			http.post(`${automationsPath(":organizationId")}/schedule-preview`, () =>
				validationError("schedule_time_zone", "Unknown time zone."),
			),
		);
		const dialog = await openCreateDialog(user);

		await waitFor(() => {
			expect(
				within(dialog).getByRole("combobox", { name: "Time zone" }),
			).toHaveAccessibleDescription("Unknown time zone.");
		});
		expect(
			within(dialog).getByLabelText(/^Cron expression/),
		).toHaveAccessibleDescription(
			"Five fields: minute, hour, day of month, month, day of week.",
		);
	});

	it("names the empty model catalog on the model selector", async () => {
		const user = userEvent.setup();
		setupEditor();
		server.use(
			http.get("/api/v2/organizations/:organizationId/chats/models", () =>
				HttpResponse.json({
					models: [],
					providers: [],
					unsupported_providers: [],
				}),
			),
		);
		const dialog = await openCreateDialog(user);

		await user.click(
			within(dialog).getByRole("radio", { name: "New chat each run" }),
		);
		expect(
			await within(dialog).findByRole("combobox", {
				name: "Model, No Models Configured",
			}),
		).toBeDisabled();
	});

	it("shows upcoming runs in UTC when the browser does not know the zone", async () => {
		const user = userEvent.setup();
		setupEditor({
			automations: [{ ...mockAutomation, schedule_time_zone: "Mars/Olympus" }],
		});

		await user.click(
			await screen.findByRole("button", {
				name: `Edit ${MockChatAutomation.name}`,
			}),
		);
		const dialog = await screen.findByRole("dialog");
		await waitFor(() => {
			expect(
				within(dialog).getByRole("region", { name: "Upcoming runs" }),
			).toHaveTextContent(/9:30 AM UTC/);
		});
	});

	it("keeps the editor open while a save is pending", async () => {
		const user = userEvent.setup();
		setupEditor();
		let releaseSave = () => {};
		server.use(
			http.post(automationsPath(":organizationId"), async () => {
				await new Promise<void>((resolve) => {
					releaseSave = resolve;
				});
				return HttpResponse.json(
					{ automation: mockAutomation },
					{ status: 201 },
				);
			}),
		);
		const dialog = await openCreateDialog(user);
		await pickChat(user, dialog, mockTargetChat.title);
		await user.click(within(dialog).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(
				within(dialog).getByRole("button", { name: "Cancel" }),
			).toBeDisabled();
		});
		expect(within(dialog).getByLabelText(/^Name/)).toBeDisabled();
		await user.keyboard("{Escape}");
		expect(screen.getByRole("dialog")).toBe(dialog);
		releaseSave();
	});

	it("keeps the reasoning effort when the same model is picked again", async () => {
		const user = userEvent.setup();
		const { updateBodies } = setupEditor({
			automations: [
				{
					...mockAutomation,
					target_mode: "new_chat",
					target_chat_id: undefined,
					new_chat_model_config_id: mockModel.id,
					reasoning_effort: "low",
				},
			],
		});

		await user.click(
			await screen.findByRole("button", {
				name: `Edit ${MockChatAutomation.name}`,
			}),
		);
		const dialog = await screen.findByRole("dialog");
		await user.click(
			await within(dialog).findByRole("combobox", {
				name: `Model, ${mockModel.display_name}`,
			}),
		);
		await user.click(
			await screen.findByRole("option", {
				name: new RegExp(mockModel.display_name),
			}),
		);
		await user.keyboard("{Escape}");
		await user.click(within(dialog).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(screen.queryByRole("dialog")).toBeNull();
		});
		expect(updateBodies).toEqual([]);
	});

	it.each([
		{ label: "a schedule", automation: mockAutomation },
		{ label: "a webhook", automation: mockWebhookAutomation },
	])(
		"sends only the changed fields when editing $label",
		async ({ automation }) => {
			const user = userEvent.setup();
			const { updateBodies } = setupEditor({ automations: [automation] });

			await user.click(
				await screen.findByRole("button", { name: `Edit ${automation.name}` }),
			);
			const dialog = await screen.findByRole("dialog");
			await pickChat(user, dialog, mockOtherChat.title);
			await user.click(within(dialog).getByRole("button", { name: "Save" }));

			await waitFor(() => {
				expect(updateBodies).toEqual([{ target_chat_id: mockOtherChat.id }]);
			});
		},
	);

	it.each([
		{ status: 404, label: "Chat not found" },
		{ status: 500, label: "Could not load chat" },
	])(
		"labels a target chat that fails with $status as $label",
		async ({ status, label }) => {
			const user = userEvent.setup();
			setupEditor();
			server.use(
				http.get("/api/v2/chats/:chatId", () =>
					HttpResponse.json({ message: "Chat error." }, { status }),
				),
			);

			await user.click(
				await screen.findByRole("button", {
					name: `Edit ${MockChatAutomation.name}`,
				}),
			);
			const dialog = await screen.findByRole("dialog");
			await waitFor(() => {
				expect(
					within(dialog).getByRole("button", { name: "Chat" }),
				).toHaveTextContent(label);
			});
		},
	);

	it.each([
		{
			failure: "forbidden",
			response: () =>
				HttpResponse.json(
					{ message: "Only the owner of a chat automation can change it." },
					{ status: 403 },
				),
			alertText: "Only the owner of a chat automation can change it.",
		},
		{
			// A validation on a field the form does not render has no field to
			// show on, so the alert lists it.
			failure: "rejected on a field the form does not show",
			response: () => validationError("kind", "Kind cannot change."),
			alertText: "Invalid chat automation.kind: Kind cannot change.",
		},
	])(
		"keeps the editor open with the server message when saving is $failure",
		async ({ response, alertText }) => {
			const user = userEvent.setup();
			setupEditor();
			server.use(
				http.patch(
					`${automationsPath(":organizationId")}/:automationId`,
					response,
				),
			);

			await user.click(
				await screen.findByRole("button", {
					name: `Edit ${MockChatAutomation.name}`,
				}),
			);
			const dialog = await screen.findByRole("dialog");
			const name = within(dialog).getByLabelText(/^Name/);
			await user.clear(name);
			await user.type(name, "Renamed");
			await user.click(within(dialog).getByRole("button", { name: "Save" }));

			const alert = await within(dialog).findByRole("alert");
			expect(alert.textContent).toBe(alertText);
			expect(within(dialog).getByLabelText(/^Name/)).toHaveValue("Renamed");
		},
	);

	it("returns focus to the button that opened the editor", async () => {
		const user = userEvent.setup();
		setupEditor();
		const newButton = await screen.findByRole("button", {
			name: "New automation",
		});

		await user.click(newButton);
		await screen.findByRole("dialog");
		await user.keyboard("{Escape}");

		await waitFor(() => {
			expect(newButton).toHaveFocus();
		});
	});
});

const webhookSecret = "test-webhook-secret-5f3a";

const rotatePath = `${automationsPath(MockDefaultOrganization.id)}/${mockWebhookAutomation.id}/secret/rotate`;

// A stray click outside keeps the secret; Done drops every copy of it.
const dismissSecret = async (
	user: ReturnType<typeof userEvent.setup>,
	secretDialog: HTMLElement,
	queryClient: QueryClient,
	mutationStates: readonly string[],
) => {
	expect(secretDialog).toHaveTextContent(webhookSecret);
	// The modal sets pointer-events: none on the body, but Radix still sees this outside press.
	await userEvent
		.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
		.pointer({ keys: "[MouseLeft]", target: document.body });
	await user.click(screen.getByRole("button", { name: "Done" }));
	await waitFor(() => {
		expect(document.body.innerHTML).not.toContain(webhookSecret);
		expect(
			JSON.stringify([
				queryClient
					.getQueryCache()
					.getAll()
					.map((query) => query.state.data),
				mutationStates,
			]),
		).not.toContain(webhookSecret);
	});
	expect(JSON.stringify(Object.entries(localStorage))).not.toContain(
		webhookSecret,
	);
	expect(JSON.stringify(Object.entries(sessionStorage))).not.toContain(
		webhookSecret,
	);
};

const openWebhookEditor = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(
		await screen.findByRole("button", {
			name: `Edit ${mockWebhookAutomation.name}`,
		}),
	);
	return screen.findByRole("dialog");
};

const confirmRotate = async (
	user: ReturnType<typeof userEvent.setup>,
	dialog: HTMLElement,
	button: "Cancel" | "Rotate secret",
) => {
	await user.click(
		within(dialog).getByRole("button", { name: "Rotate secret" }),
	);
	const confirm = await screen.findByRole("dialog", {
		name: "Rotate the webhook secret?",
	});
	await user.click(within(confirm).getByRole("button", { name: button }));
};

describe("AgentAutomationsPage webhooks", { timeout: 15_000 }, () => {
	it("creates a single-use webhook and shows its secret only once", async () => {
		const user = userEvent.setup();
		const { queryClient, mutationStates, createBodies } = setupEditor();
		server.use(
			http.post(automationsPath(":organizationId"), async ({ request }) => {
				createBodies.push(await request.json());
				return HttpResponse.json(
					{ automation: mockWebhookAutomation, webhook_secret: webhookSecret },
					{ status: 201 },
				);
			}),
		);
		const dialog = await openCreateDialog(user);

		await user.click(within(dialog).getByRole("radio", { name: "Webhook" }));
		await user.click(within(dialog).getByRole("radio", { name: "Single-use" }));
		await pickChat(user, dialog, MockChat.title);
		await user.click(within(dialog).getByRole("button", { name: "Save" }));

		const secretDialog = await screen.findByRole("dialog", {
			name: "Copy the webhook secret",
		});
		expect(createBodies).toEqual([
			{
				name: "Standup",
				kind: "webhook",
				target_mode: "existing_chat",
				prompt: "Summarize yesterday.",
				webhook_use: "single",
				target_chat_id: MockChat.id,
				when_busy: "queue",
			},
		]);
		expect(
			within(secretDialog).getByRole("button", { name: "Done" }),
		).toHaveFocus();
		await dismissSecret(user, secretDialog, queryClient, mutationStates);
		await waitFor(() => {
			expect(
				screen.getByRole("button", { name: "New automation" }),
			).toHaveFocus();
		});
	});

	const scheduleBody = {
		kind: "schedule",
		schedule_cron: "0 9 * * *",
		schedule_time_zone: browserTimeZone,
		when_busy: "skip",
	};
	it.each([
		{
			label: "switching to webhook and back",
			whenBusy: "",
			triggers: ["Webhook", "Schedule"],
			body: scheduleBody,
		},
		{
			label: "picking When busy, then switching to webhook and back",
			whenBusy: "Queue the prompt",
			triggers: ["Webhook", "Schedule"],
			body: { ...scheduleBody, when_busy: "queue" },
		},
		{
			label: "picking webhook without a use",
			whenBusy: "",
			triggers: ["Webhook"],
			body: { kind: "webhook", webhook_use: "multi", when_busy: "queue" },
		},
	])(
		"sends the create body after $label",
		async ({ triggers, whenBusy, body }) => {
			const user = userEvent.setup();
			const { createBodies } = setupEditor();
			const dialog = await openCreateDialog(user);

			if (whenBusy) {
				await user.click(
					within(dialog).getByRole("combobox", { name: "When busy" }),
				);
				await user.click(await screen.findByRole("option", { name: whenBusy }));
			}
			for (const trigger of triggers) {
				await user.click(within(dialog).getByRole("radio", { name: trigger }));
			}
			await pickChat(user, dialog, MockChat.title);
			await user.click(within(dialog).getByRole("button", { name: "Save" }));

			await waitFor(() => {
				expect(createBodies).toEqual([
					{
						name: "Standup",
						target_mode: "existing_chat",
						prompt: "Summarize yesterday.",
						target_chat_id: MockChat.id,
						...body,
					},
				]);
			});
		},
	);

	it("rotates the secret only after confirmation and shows it once", async () => {
		const user = userEvent.setup();
		const { queryClient, mutationStates } = setupEditor();
		const rotateRequests: Request[] = [];
		let releaseRotate = () => {};
		server.use(
			http.get(automationsPath(":organizationId"), () =>
				HttpResponse.json([mockWebhookAutomation]),
			),
			http.post(rotatePath, async ({ request }) => {
				rotateRequests.push(request);
				await new Promise<void>((resolve) => {
					releaseRotate = resolve;
				});
				return HttpResponse.json({
					webhook_secret: webhookSecret,
					webhook_secret_version: 2,
				});
			}),
		);
		const dialog = await openWebhookEditor(user);

		await confirmRotate(user, dialog, "Cancel");
		expect(rotateRequests).toEqual([]);
		await confirmRotate(user, dialog, "Rotate secret");
		await waitFor(() => {
			expect(rotateRequests).toHaveLength(1);
		});
		// The name includes the spinner's title while rotating.
		const rotatingButton = within(dialog).getByRole("button", {
			name: /Rotate secret/,
		});
		expect(rotatingButton).toHaveFocus();
		await user.click(rotatingButton);
		expect(
			screen.queryByRole("dialog", { name: "Rotate the webhook secret?" }),
		).toBeNull();
		// The editor stays open mid-rotation so focus can return to the button.
		await user.keyboard("{Escape}");
		releaseRotate();

		const secretDialog = await screen.findByRole("dialog", {
			name: "Copy the webhook secret",
		});
		await dismissSecret(user, secretDialog, queryClient, mutationStates);
		await waitFor(() => {
			expect(
				within(dialog).getByRole("button", { name: "Rotate secret" }),
			).toHaveFocus();
		});
	});

	it("shows the server message when rotating the secret is forbidden", async () => {
		const user = userEvent.setup();
		setupEditor();
		let rotatePosts = 0;
		server.use(
			http.get(automationsPath(":organizationId"), () =>
				HttpResponse.json([mockWebhookAutomation]),
			),
			http.post(rotatePath, () => {
				rotatePosts++;
				return HttpResponse.json(
					{ message: "Only the owner of a chat automation can change it." },
					{ status: 403 },
				);
			}),
		);
		const dialog = await openWebhookEditor(user);

		await confirmRotate(user, dialog, "Rotate secret");

		expect(await within(dialog).findByRole("alert")).toHaveTextContent(
			/^Only the owner of a chat automation can change it\.$/,
		);
		expect(rotatePosts).toBe(1);
	});
});
