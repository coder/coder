import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { TerminalClientSessionContext } from "../context/TerminalClientSessionContext";
import { TabPanelStateContext } from "./ChatsSidebar/tabs/SidebarTabView";
import { TerminalPanel } from "./TerminalPanel";

vi.stubGlobal("jest", vi);
await import("jest-canvas-mock");
const { default: WS } = await import("jest-websocket-mock");

// The mock server matches URLs without the query string.
const createTerminalWebSocket = () => {
	return new WS(
		`ws://${location.host}/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty`,
	);
};

// The server closes normally once the agent ends the session.
const normalClosure = { code: 1000, reason: "", wasClean: true };

const CommandTerminalTab: React.FC = () => {
	const [isActive, setIsActive] = useState(true);
	const [operatingSystem, setOperatingSystem] = useState("linux");
	return (
		<TerminalClientSessionContext value="0123456789abcdef0123456789abcdef">
			<button type="button" onClick={() => setIsActive(!isActive)}>
				Switch tab
			</button>
			<button type="button" onClick={() => setOperatingSystem("windows")}>
				Change agent
			</button>
			<TabPanelStateContext
				value={{ isActive, isPanelOpen: true, isExpanded: false }}
			>
				<TerminalPanel
					chatId="b5a8832c-72db-4679-8393-9a48dff20a20"
					initialCommand="echo hello"
					isPending={false}
					onReady={() => {}}
					workspace={MockWorkspace}
					workspaceAgent={{
						...MockWorkspaceAgent,
						operating_system: operatingSystem,
					}}
				/>
			</TabPanelStateContext>
		</TerminalClientSessionContext>
	);
};

const renderEndedCommandTerminal = async () => {
	const ws = createTerminalWebSocket();
	renderWithAuth(<CommandTerminalTab />);
	await ws.nextMessage;
	ws.close(normalClosure);
	const runAgain = await screen.findByRole("button", { name: "Run again" });
	const nextWs = createTerminalWebSocket();
	const onReconnect = vi.fn();
	nextWs.on("connection", onReconnect);
	return { runAgain, nextWs, onReconnect };
};

describe("TerminalPanel", () => {
	afterEach(async () => {
		vi.useRealTimers();
		await WS.clean();
	});

	it("runs an exited command again when asked", async () => {
		const { runAgain, nextWs } = await renderEndedCommandTerminal();

		await userEvent.click(runAgain);

		await nextWs.nextMessage;
		expect(nextWs.server.clients()[0].url).toContain("command=echo+hello");
	});

	it("does not take focus when its command exits", async () => {
		const ws = createTerminalWebSocket();
		renderWithAuth(<CommandTerminalTab />);
		await ws.nextMessage;
		await waitFor(() =>
			expect(
				screen.getByRole("textbox", { name: "Terminal input" }),
			).toHaveFocus(),
		);
		const elsewhere = screen.getByRole("button", { name: "Change agent" });
		elsewhere.focus();

		ws.close(normalClosure);
		await screen.findByRole("button", { name: "Run again" });
		await new Promise(requestAnimationFrame);

		expect(elsewhere).toHaveFocus();
	});

	it.each([
		{
			name: "the tab is hidden past the idle detach and shown again",
			change: async (user: ReturnType<typeof userEvent.setup>) => {
				const switchTab = screen.getByRole("button", { name: "Switch tab" });
				await user.click(switchTab);
				await vi.advanceTimersByTimeAsync(60_000);
				await user.click(switchTab);
			},
		},
		{
			name: "the terminal's connection settings change",
			change: async (user: ReturnType<typeof userEvent.setup>) => {
				await user.click(screen.getByRole("button", { name: "Change agent" }));
			},
		},
	])("does not run an exited command again when $name", async ({ change }) => {
		const { onReconnect } = await renderEndedCommandTerminal();
		// The idle detach and the mock server's accept both run on timers.
		vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
		const user = userEvent.setup({
			advanceTimers: vi.advanceTimersByTimeAsync,
		});

		await change(user);
		await vi.advanceTimersByTimeAsync(60_000);

		expect(onReconnect).not.toHaveBeenCalled();
	});
});
