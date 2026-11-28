import { screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import RightPanelTabPopoutPage from "./RightPanelTabPopoutPage";
import {
	type TabPopoutMessage,
	tabPopoutChannelName,
} from "./utils/rightPanelTabPopout";
import { savePersistedRightPanelTabs } from "./utils/rightPanelTabStorage";
import type { UserRightPanelTab } from "./utils/rightPanelTabs";

// The watch opens a socket the page under test has no use for.
vi.mock("./components/ChatConversation/useWorkspaceWatch", () => ({
	useWorkspaceWatch: () => {},
}));

const previewTab: UserRightPanelTab = {
	id: "port-3000",
	kind: "port",
	label: "Preview :3000",
	agentId: MockWorkspaceAgent.id,
	port: 3000,
	protocol: "http",
};

function renderPage(tabId = previewTab.id) {
	vi.spyOn(API.experimental, "getChat").mockResolvedValue({
		...MockChat,
		workspace_id: MockWorkspace.id,
		agent_id: MockWorkspaceAgent.id,
	});
	server.use(
		http.get("/api/v2/experiments", () =>
			HttpResponse.json(["chat-ui-annotations"]),
		),
	);
	return renderWithAuth(<RightPanelTabPopoutPage />, {
		path: "/agents/:agentId/tabs/:tabId",
		route: `/agents/${MockChat.id}/tabs/${tabId}`,
	});
}

describe("RightPanelTabPopoutPage", () => {
	afterEach(() => {
		localStorage.clear();
		vi.restoreAllMocks();
	});

	it("shows the persisted tab, annotating, and relays sends to the chat", async () => {
		savePersistedRightPanelTabs(MockChat.id, [previewTab]);
		const chatPage = new BroadcastChannel(tabPopoutChannelName(previewTab.id));
		const fromWindow: TabPopoutMessage[] = [];
		chatPage.addEventListener("message", (event) => {
			const message = event.data as TabPopoutMessage;
			fromWindow.push(message);
			if (message.type === "send") {
				chatPage.postMessage({
					type: "send-result",
					id: message.id,
					result: "sent",
				} satisfies TabPopoutMessage);
			}
		});
		try {
			renderPage();
			const frame =
				await screen.findByTitle<HTMLIFrameElement>("Preview :3000");
			// The panel runs as the tab's own window: the app is sandboxed, the
			// overlay is requested up front and annotate mode is on.
			expect(frame.getAttribute("sandbox")).toContain("allow-scripts");
			expect(frame.getAttribute("sandbox")).not.toContain(
				"allow-top-navigation",
			);
			expect(new URL(frame.src).searchParams.get("coder_annotate")).toBe("1");
			expect(new URL(frame.src).hostname).toMatch(
				/^3000--a-workspace-agent--test-workspace--.*\.coder\.com$/,
			);
			expect(
				screen.getByRole("button", { name: "Stop annotating" }),
			).toHaveAttribute("aria-pressed", "true");
			await waitFor(() =>
				expect(fromWindow).toContainEqual({
					type: "popout-opened",
				} satisfies TabPopoutMessage),
			);

			// A submission from the overlay reaches the chat page's composer.
			const frameOrigin = new URL(frame.src).origin;
			const fromOverlay = (data: unknown) =>
				window.dispatchEvent(
					new MessageEvent("message", {
						data,
						origin: frameOrigin,
						source: frame.contentWindow,
					}),
				);
			fromOverlay({ type: "coder-annotator:ready" });
			fromOverlay({
				type: "coder-annotator:submit",
				page: {
					url: `${frameOrigin}/`,
					title: "App",
					viewport: { width: 800, height: 600 },
				},
				annotations: [
					{
						id: "a",
						comment: "Make this red",
						element: {
							tag: "button",
							selector: "#save",
							classes: [],
							openingTag: '<button id="save">',
							rect: { x: 1, y: 2, width: 3, height: 4 },
						},
					},
				],
			});
			await waitFor(() =>
				expect(
					fromWindow.find((message) => message.type === "send"),
				).toBeDefined(),
			);
			const send = fromWindow.find((message) => message.type === "send");
			expect(send?.type === "send" && send.message).toContain(
				"> Make this red",
			);

			// The chat page can call the window back in.
			const close = vi.spyOn(window, "close").mockImplementation(() => {});
			chatPage.postMessage({ type: "bring-back" } satisfies TabPopoutMessage);
			await waitFor(() => expect(close).toHaveBeenCalled());
		} finally {
			chatPage.close();
		}
	});

	it("says so when the tab is no longer open in the chat", async () => {
		renderPage("port-gone");
		expect(
			await screen.findByText("This tab is no longer open in the chat."),
		).toBeInTheDocument();
	});

	it("only shows port previews", async () => {
		savePersistedRightPanelTabs(MockChat.id, [
			{
				id: "terminal-1",
				kind: "terminal",
				label: "Terminal",
				reconnectionToken: "token",
			},
		]);
		renderPage("terminal-1");
		expect(
			await screen.findByText("This tab cannot be shown in a separate window."),
		).toBeInTheDocument();
	});
});
