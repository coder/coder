import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Chat, ChatAutomation, ChatModel } from "#/api/typesGenerated";
import { MockChat, MockChatAutomation } from "#/testHelpers/chatEntities";
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

const automationsPath = (organizationId: string) =>
	`/api/experimental/organizations/${organizationId}/chat-automations`;

const setup = ({ experiments = ["chat-automations"] } = {}) => {
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
			HttpResponse.json([mockAutomation]),
		),
	);
	renderWithAuth(<AgentAutomationsPage />);
	return requests;
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
		const requests = setup();

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
		const requests = setup({ experiments: [] });

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
			const requests = setup();
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
		const requests = setup();

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
		const requests = setup();
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

const otherChat: Chat = { ...MockChat, id: "chat-2", title: "Release notes" };

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

const setupEditor = () => {
	const previewBodies: unknown[] = [];
	const createBodies: unknown[] = [];
	const updateBodies: unknown[] = [];
	const requests = setup();
	server.use(
		http.get("/api/v2/chats", ({ request }) => {
			requests.push(request);
			return HttpResponse.json([MockChat, otherChat]);
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
	return { requests, previewBodies, createBodies, updateBodies };
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
			).toContain('title:"Rele" archived:false');
		});
		await user.click(
			await screen.findByRole("option", { name: otherChat.title }),
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
					target_chat_id: otherChat.id,
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
		// The slider starts at the effective default ("high"), so moving it
		// left picks "low".
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
		const previewErrors = within(dialog).getAllByText(
			"Expected exactly five fields.",
		);
		expect(previewErrors).toHaveLength(1);
		expect(previewErrors[0].closest("[aria-live]")).toHaveAttribute(
			"aria-live",
			"polite",
		);

		await pickChat(user, dialog, MockChat.title);
		await user.click(within(dialog).getByRole("button", { name: "Save" }));
		await waitFor(() => {
			expect(cron).toHaveAccessibleDescription(
				expect.stringContaining("Must be a valid cron expression."),
			);
		});
		expect(within(dialog).queryByRole("alert")).toBeNull();

		// The save error no longer applies once the user edits the cron.
		await user.type(cron, "x");
		expect(cron).toHaveAccessibleDescription(
			expect.not.stringContaining("Must be a valid cron expression."),
		);
	});

	it("sends only the changed fields when editing", async () => {
		const user = userEvent.setup();
		const { updateBodies } = setupEditor();

		await user.click(
			await screen.findByRole("button", {
				name: `Edit ${MockChatAutomation.name}`,
			}),
		);
		const dialog = await screen.findByRole("dialog");
		await pickChat(user, dialog, otherChat.title);
		await user.click(within(dialog).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(updateBodies).toEqual([{ target_chat_id: otherChat.id }]);
		});
	});

	it("keeps the editor open with the server message when saving is forbidden", async () => {
		const user = userEvent.setup();
		setupEditor();
		const message = "Only the owner of a chat automation can change it.";
		server.use(
			http.patch(`${automationsPath(":organizationId")}/:automationId`, () =>
				HttpResponse.json({ message }, { status: 403 }),
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
		expect(alert.textContent).toBe(message);
		expect(within(dialog).getByLabelText(/^Name/)).toHaveValue("Renamed");
	});

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
