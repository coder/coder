import { waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import TerminalPage from "./TerminalPage";

const reconnectToken = "terminal-page-test-reconnect-token";

vi.mock("uuid", () => ({
	v4: () => "terminal-page-test-reconnect-token",
}));
vi.stubGlobal("jest", vi);
await import("jest-canvas-mock");
const { default: WS } = await import("jest-websocket-mock");

Object.defineProperty(window, "matchMedia", {
	writable: true,
	value: vi.fn().mockImplementation((query: string) => ({
		matches: false,
		media: query,
		onchange: null,
		addListener: vi.fn(),
		removeListener: vi.fn(),
		addEventListener: vi.fn(),
		removeEventListener: vi.fn(),
		dispatchEvent: vi.fn(),
	})),
});

const createWorkspaceTerminalWebSocket = () => {
	const websocketProtocol =
		window.location.protocol === "https:" ? "wss" : "ws";
	const websocketUrl = `${websocketProtocol}://${window.location.host}/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty?reconnect=${reconnectToken}&height=24&width=80`;

	return new WS(websocketUrl);
};

const renderTerminal = async (
	route = `/${MockUserOwner.username}/${MockWorkspace.name}/terminal`,
) => {
	const utils = renderWithAuth(<TerminalPage />, {
		route,
		path: "/:username/:workspace/terminal",
	});
	await waitFor(() => {
		// To avoid 'act' errors during testing, we ensure the component is
		// completely rendered without any outstanding state updates. This is
		// accomplished by incorporating a 'data-status' attribute into the
		// component. We then observe this attribute for any changes, as we cannot
		// rely on other screen elements to indicate completion.
		const wrapper =
			utils.container.querySelector<HTMLDivElement>("[data-status]")!;
		expect(wrapper.dataset.status).not.toBe("initializing");
	});
	return utils;
};

const expectTerminalText = (container: HTMLElement, text: string) => {
	return waitFor(
		() => {
			const elements = container.getElementsByClassName("xterm-rows");
			if (elements.length === 0) {
				throw new Error("no xterm-rows");
			}
			const row = elements[0] as HTMLDivElement;
			if (!row.textContent) {
				throw new Error("no text content");
			}
			expect(row.textContent).toContain(text);
		},
		{ timeout: 5_000 },
	);
};

describe("TerminalPage", () => {
	afterEach(async () => {
		vi.restoreAllMocks();
		await WS.clean();
	});

	it("loads the right workspace data", async () => {
		vi.spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockWorkspace,
		);
		createWorkspaceTerminalWebSocket();
		await renderTerminal(
			`/${MockUserOwner.username}/${MockWorkspace.name}/terminal`,
		);
		await waitFor(() => {
			expect(API.getWorkspaceByOwnerAndName).toHaveBeenCalledWith(
				MockUserOwner.username,
				MockWorkspace.name,
				{ include_deleted: true },
			);
		});
	});

	it("shows an error if fetching workspace fails", async () => {
		server.use(
			http.get("/api/v2/users/:userId/workspace/:workspaceName", () => {
				return HttpResponse.json({ id: "workspace-id" }, { status: 500 });
			}),
		);

		const { container } = await renderTerminal();

		await expectTerminalText(container, "Unable to fetch workspace: ");
	});

	it("shows reconnect message when websocket fails", async () => {
		server.use(
			http.get("/api/v2/workspaceagents/:agentId/pty", () => {
				return HttpResponse.json({}, { status: 500 });
			}),
		);

		const { container } = await renderTerminal();

		await waitFor(() => {
			expect(container.textContent).toContain("Trying to connect...");
		});
	});

	it("renders data from the backend", async () => {
		const ws = createWorkspaceTerminalWebSocket();
		const text = "something to render";

		const { container } = await renderTerminal();
		ws.send(text);

		await expectTerminalText(container, text);
	});

	// Ideally we could just pass the correct size in the web socket URL without
	// having to resize separately afterward (and then we could delete also this
	// test), but we need the initial resize message to have something to wait for
	// in the other tests since ws.connected appears to pause React updates.  So
	// for now the initial resize message (and this test) are here to stay.
	it("resizes on connect", async () => {
		const ws = createWorkspaceTerminalWebSocket();
		const resizeMessage = ws.nextMessage;

		await renderTerminal();

		const msg = await resizeMessage;
		const req = JSON.parse(new TextDecoder().decode(msg as Uint8Array));
		expect(req.height).toBeGreaterThan(0);
		expect(req.width).toBeGreaterThan(0);
	});

	it("supports workspace.agent syntax", async () => {
		const ws = createWorkspaceTerminalWebSocket();
		const text = "something to render";

		const { container } = await renderTerminal(
			`/some-user/${MockWorkspace.name}.${MockWorkspaceAgent.name}/terminal`,
		);

		ws.send(text);
		await expectTerminalText(container, text);
	});

	it("supports shift+enter", async () => {
		const ws = createWorkspaceTerminalWebSocket();
		const initialResizeMessage = ws.nextMessage;

		const { container } = await renderTerminal();
		await initialResizeMessage;

		const msg = ws.nextMessage;
		const terminal = container.getElementsByClassName("xterm");
		await userEvent.type(terminal[0], "{Shift>}{Enter}{/Shift}");
		const req = JSON.parse(new TextDecoder().decode((await msg) as Uint8Array));
		expect(req.data).toBe("\x1b\r");
	});
});
