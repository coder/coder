import "jest-canvas-mock";
import { waitFor } from "@testing-library/react";
import { API } from "api/api";
import WS from "jest-websocket-mock";
import { http, HttpResponse } from "msw";
import {
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
} from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import TerminalPage, { Language } from "./TerminalPage";

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
		expect(wrapper.dataset.state).not.toBe("initializing");
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
	afterEach(() => {
		WS.clean();
	});

	it("loads the right workspace data", async () => {
		jest
			.spyOn(API, "getWorkspaceByOwnerAndName")
			.mockResolvedValue(MockWorkspace);
		new WS(
			`ws://localhost/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty`,
		);
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

		await expectTerminalText(container, Language.workspaceErrorMessagePrefix);
	});

	it("shows an error if the websocket fails", async () => {
		server.use(
			http.get("/api/v2/workspaceagents/:agentId/pty", () => {
				return HttpResponse.json({}, { status: 500 });
			}),
		);

		const { container } = await renderTerminal();

		await expectTerminalText(container, Language.websocketErrorMessagePrefix);
	});

	it("renders data from the backend", async () => {
		const ws = new WS(
			`ws://localhost/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty`,
		);
		const text = "something to render";

		const { container } = await renderTerminal();
		// Ideally we could use ws.connected but that seems to pause React updates.
		// For now, wait for the initial resize message instead.
		await ws.nextMessage;
		ws.send(text);

		await expectTerminalText(container, text);
	});

	// Ideally we could just pass the correct size in the web socket URL without
	// having to resize separately afterward (and then we could delete also this
	// test), but we need the initial resize message to have something to wait for
	// in the other tests since ws.connected appears to pause React updates.  So
	// for now the initial resize message (and this test) are here to stay.
	it("resizes on connect", async () => {
		const ws = new WS(
			`ws://localhost/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty`,
		);

		await renderTerminal();

		const msg = await ws.nextMessage;
		const req = JSON.parse(new TextDecoder().decode(msg as Uint8Array));
		expect(req.height).toBeGreaterThan(0);
		expect(req.width).toBeGreaterThan(0);
	});

	it("supports workspace.agent syntax", async () => {
		const ws = new WS(
			`ws://localhost/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty`,
		);
		const text = "something to render";

		const { container } = await renderTerminal(
			`/some-user/${MockWorkspace.name}.${MockWorkspaceAgent.name}/terminal`,
		);

		// Ideally we could use ws.connected but that seems to pause React updates.
		// For now, wait for the initial resize message instead.
		await ws.nextMessage;
		ws.send(text);
		await expectTerminalText(container, text);
	});
});
