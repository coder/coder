import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChatAutomation } from "#/api/typesGenerated";
import { MockChat, MockChatAutomation } from "#/testHelpers/chatEntities";
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

const setup = ({
	experiments = ["chat-automations"],
	automations = [mockAutomation],
}: {
	experiments?: string[];
	automations?: ChatAutomation[];
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
	vi.restoreAllMocks();
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

	it("shows the error of a failed toggle after another row was toggled", async () => {
		const user = userEvent.setup();
		const toastError = vi.spyOn(toast, "error");
		const secondAutomation: ChatAutomation = {
			...mockAutomation,
			id: "second-automation",
			name: "Second automation",
		};
		setup({ automations: [mockAutomation, secondAutomation] });
		let failFirstUpdate = () => {};
		const firstUpdateFailed = new Promise<void>((resolve) => {
			failFirstUpdate = resolve;
		});
		server.use(
			http.patch(
				`${automationsPath(MockDefaultOrganization.id)}/${mockAutomation.id}`,
				async () => {
					await firstUpdateFailed;
					return HttpResponse.json(
						{ message: "The first update failed." },
						{ status: 400 },
					);
				},
			),
			http.patch(
				`${automationsPath(MockDefaultOrganization.id)}/${secondAutomation.id}`,
				() => HttpResponse.json({ ...secondAutomation, enabled: false }),
			),
		);

		await user.click(
			await screen.findByRole("switch", {
				name: `Enable ${mockAutomation.name}`,
			}),
		);
		await user.click(
			await screen.findByRole("switch", {
				name: `Enable ${secondAutomation.name}`,
			}),
		);
		failFirstUpdate();

		await waitFor(() => {
			expect(toastError).toHaveBeenCalledWith(
				"The first update failed.",
				expect.anything(),
			);
		});
	});

	it("sends the Run now request", async () => {
		const user = userEvent.setup();
		const requests = setup();
		const runPath = `${automationsPath(MockDefaultOrganization.id)}/${mockAutomation.id}/runs`;
		server.use(
			http.post(runPath, ({ request }) => {
				requests.push(request);
				return HttpResponse.json(
					{ message: "The target chat is busy." },
					{ status: 409 },
				);
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
	});

	it("requests an automation's archived and active chats", async () => {
		const user = userEvent.setup();
		const requests = setup();

		await user.click(
			await screen.findByRole("button", {
				name: `View chats ${mockAutomation.name}`,
			}),
		);

		await waitFor(() => {
			expect(requestPaths(requests)).toContain(
				`GET /api/v2/chats?automation_id=${mockAutomation.id}&q=archived%3Aany&limit=25&offset=0`,
			);
		});
	});

	it("returns focus to View chats when the chats dialog closes", async () => {
		const user = userEvent.setup();
		setup();

		const viewChats = await screen.findByRole("button", {
			name: `View chats ${mockAutomation.name}`,
		});
		viewChats.focus();
		await user.keyboard("{Enter}");
		await screen.findByRole("dialog", {
			name: `Chats for ${mockAutomation.name}`,
		});
		await user.keyboard("{Escape}");

		await waitFor(() => {
			expect(viewChats).toHaveFocus();
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
				`GET /api/v2/chats?automation_id=${mockAutomation.id}&q=archived%3Aany&limit=25&offset=25`,
			);
		});
	});
});
