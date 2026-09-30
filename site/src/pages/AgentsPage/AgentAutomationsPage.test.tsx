import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it } from "vitest";
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

const automation: ChatAutomation = {
	...MockChatAutomation,
	organization_id: MockDefaultOrganization.id,
	target_chat_id: MockChat.id,
};

const automationsPath = (organizationId: string) =>
	`/api/experimental/organizations/${organizationId}/chat-automations`;

/** Serves the page's dependencies and records the requests it sends. */
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
			HttpResponse.json([automation]),
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
				`${automationsPath(MockDefaultOrganization.id)}/${automation.id}`,
				async ({ request }) => {
					body = await request.json();
					return HttpResponse.json({ ...automation, enabled: false });
				},
			),
		);

		await user.click(
			await screen.findByRole("switch", { name: `Enable ${automation.name}` }),
		);

		await waitFor(() => {
			expect(body).toEqual({ enabled: false });
		});
	});

	it.each([
		[409, "The target chat is busy."],
		[429, "The automation used up its share of the chat queue."],
	])("shows a %i Run now response as an error", async (status, message) => {
		const user = userEvent.setup();
		setup();
		server.use(
			http.post(
				`${automationsPath(MockDefaultOrganization.id)}/${automation.id}/runs`,
				() => HttpResponse.json({ message }, { status }),
			),
		);

		await user.click(await screen.findByRole("button", { name: "Run now" }));

		const alert = await screen.findByRole("alert");
		expect(alert.textContent).toContain(message);
		expect(alert.textContent).toContain(automation.name);
	});

	it("lists an automation's chats with the automation filter", async () => {
		const user = userEvent.setup();
		const requests = setup();

		await user.click(await screen.findByRole("button", { name: "View chats" }));

		await screen.findByRole("dialog");
		await waitFor(() => {
			expect(requestPaths(requests)).toContain(
				`GET /api/v2/chats?automation_id=${automation.id}&limit=25`,
			);
		});
	});
});
