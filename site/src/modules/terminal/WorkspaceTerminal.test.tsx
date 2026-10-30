import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import "#/testHelpers/canvas";
import { WorkspaceTerminal } from "./WorkspaceTerminal";

const { default: WS } = await import("jest-websocket-mock");

const agentId = "workspace-terminal-test-agent";
const reconnectionToken = "workspace-terminal-test-reconnect";
const sessionId = "workspace-terminal-test-session";

describe("WorkspaceTerminal", () => {
	afterEach(() => {
		WS.clean();
	});

	it("reports content ready once, after the first output", async () => {
		const protocol = location.protocol === "https:" ? "wss" : "ws";
		const ws = new WS(
			`${protocol}://${location.host}/api/v2/workspaceagents/${agentId}/pty?reconnect=${reconnectionToken}&client_session_id=${sessionId}&height=24&width=80`,
		);
		const onContentReady = vi.fn();

		render(
			<WorkspaceTerminal
				agentId={agentId}
				reconnectionToken={reconnectionToken}
				sessionId={sessionId}
				onContentReady={onContentReady}
			/>,
		);
		await waitFor(() => expect(ws.server.clients()).toHaveLength(1));
		await ws.nextMessage;
		expect(onContentReady).not.toHaveBeenCalled();

		ws.send("first output");
		await waitFor(() => expect(onContentReady).toHaveBeenCalledTimes(1));

		ws.send("more output");
		await new Promise((resolve) => requestAnimationFrame(resolve));
		await new Promise((resolve) => requestAnimationFrame(resolve));
		expect(onContentReady).toHaveBeenCalledTimes(1);
	});

	it("does not take focus when autoFocus is off", async () => {
		render(
			<WorkspaceTerminal
				agentId={agentId}
				reconnectionToken={reconnectionToken}
				sessionId={sessionId}
				autoFocus={false}
				loading
			/>,
		);

		await screen.findAllByRole("textbox", { name: "Terminal input" });
		expect(document.activeElement).toBe(document.body);
	});
});
