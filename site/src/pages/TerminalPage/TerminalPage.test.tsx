import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Workspace } from "#/api/typesGenerated";
import {
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
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
	const websocketProtocol = location.protocol === "https:" ? "wss" : "ws";
	const websocketUrl = `${websocketProtocol}://${location.host}/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty?reconnect=${reconnectToken}&height=24&width=80`;

	return new WS(websocketUrl);
};

// Renders the terminal page and waits for the terminal to finish
// initializing (i.e. the WebSocket connection is established). Do not
// use this for tests where the terminal stays in loading state, such
// as when a command confirmation dialog is blocking the connection.
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

// Renders the terminal page without waiting for the terminal to leave
// the "initializing" state. Use this for tests where a confirmation
// dialog keeps the terminal in loading state until the user acts.
const renderTerminalRaw = (
	route = `/${MockUserOwner.username}/${MockWorkspace.name}/terminal`,
) => {
	return renderWithAuth(<TerminalPage />, {
		route,
		path: "/:username/:workspace/terminal",
	});
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

	it("shows confirmation dialog when command param is present", async () => {
		renderTerminalRaw(
			`/${MockUserOwner.username}/${MockWorkspace.name}/terminal?command=echo+hello`,
		);
		const dialog = await screen.findByRole("dialog");
		expect(dialog).toHaveTextContent("echo hello");
		expect(
			screen.getByRole("button", { name: "Run command" }),
		).toBeInTheDocument();
	});

	it("does not show confirmation dialog when no command param", async () => {
		createWorkspaceTerminalWebSocket();
		await renderTerminal();
		expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
	});

	it("executes command after confirmation", async () => {
		const websocketProtocol =
			window.location.protocol === "https:" ? "wss" : "ws";
		const ws = new WS(
			`${websocketProtocol}://${window.location.host}/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty?reconnect=${reconnectToken}&command=echo+hello&height=24&width=80`,
		);
		renderTerminalRaw(
			`/${MockUserOwner.username}/${MockWorkspace.name}/terminal?command=echo+hello`,
		);
		await userEvent.click(
			await screen.findByRole("button", { name: "Run command" }),
		);
		await waitFor(() =>
			expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
		);
		// Verify the websocket connected and received a resize message.
		const msg = await ws.nextMessage;
		const resizeReq = JSON.parse(new TextDecoder().decode(msg as Uint8Array));
		expect(resizeReq.height).toBeGreaterThan(0);
		expect(resizeReq.width).toBeGreaterThan(0);
	});

	it("closes window on cancel", async () => {
		const closeSpy = vi.spyOn(window, "close").mockImplementation(() => {});
		renderTerminalRaw(
			`/${MockUserOwner.username}/${MockWorkspace.name}/terminal?command=echo+hello`,
		);
		await userEvent.click(
			await screen.findByRole("button", { name: "Cancel" }),
		);
		expect(closeSpy).toHaveBeenCalled();
	});

	it("skips confirmation dialog for trusted app commands", async () => {
		// Override the workspace response so the agent has an app with
		// a command that matches the ?app= slug.
		const appWithCommand = {
			...MockWorkspaceApp,
			slug: "my-app",
			command: "echo trusted",
		};
		const workspaceWithApp: Workspace = {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				resources: [
					{
						...MockWorkspace.latest_build.resources[0],
						agents: [
							{
								...MockWorkspaceAgent,
								apps: [appWithCommand],
							},
						],
					},
				],
			},
		};
		server.use(
			http.get("/api/v2/users/:userId/workspace/:workspaceName", () => {
				return HttpResponse.json(workspaceWithApp);
			}),
		);

		const websocketProtocol =
			window.location.protocol === "https:" ? "wss" : "ws";
		const ws = new WS(
			`${websocketProtocol}://${window.location.host}/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty?reconnect=${reconnectToken}&command=echo+trusted&height=24&width=80`,
		);
		await renderTerminal(
			`/${MockUserOwner.username}/${MockWorkspace.name}/terminal?app=my-app`,
		);

		// No dialog should appear.
		expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

		// The websocket should connect with the resolved command.
		const msg = await ws.nextMessage;
		const resizeReq = JSON.parse(new TextDecoder().decode(msg as Uint8Array));
		expect(resizeReq.height).toBeGreaterThan(0);
		expect(resizeReq.width).toBeGreaterThan(0);
	});
});
