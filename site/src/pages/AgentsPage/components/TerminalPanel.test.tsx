import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Terminal } from "@xterm/xterm";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	createWorkspaceTerminalWebSocket,
	normalClosure,
	PAST_RECONNECT_WINDOW_MS,
	setupUserWithFakeTimers,
	WS,
} from "#/modules/terminal/testHelpers";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { TerminalClientSessionContext } from "../context/TerminalClientSessionContext";
import { TabPanelStateContext } from "./ChatsSidebar/tabs/SidebarTabView";
import { TERMINAL_IDLE_DETACH_MS, TerminalPanel } from "./TerminalPanel";

type CommandTerminalTabProps = {
	initiallyEnded?: boolean;
	onEndedChange?: (ended: boolean) => void;
};

// Keeps the ended flag the way the chat page keeps it on the tab.
const CommandTerminalTab: React.FC<CommandTerminalTabProps> = ({
	initiallyEnded = false,
	onEndedChange,
}) => {
	const [isActive, setIsActive] = useState(true);
	const [operatingSystem, setOperatingSystem] = useState("linux");
	const [sessionEnded, setSessionEnded] = useState(initiallyEnded);
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
					sessionEnded={sessionEnded}
					onSessionEndedChange={(ended) => {
						setSessionEnded(ended);
						onEndedChange?.(ended);
					}}
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
	const onEndedChange = vi.fn();
	const ws = createWorkspaceTerminalWebSocket();
	renderWithAuth(<CommandTerminalTab onEndedChange={onEndedChange} />);
	await ws.nextMessage;
	ws.close(normalClosure);
	const runAgain = await screen.findByRole("button", { name: "Run again" });
	const nextWs = createWorkspaceTerminalWebSocket();
	const onReconnect = vi.fn();
	nextWs.on("connection", onReconnect);
	return { runAgain, nextWs, onReconnect, onEndedChange };
};

describe("TerminalPanel", () => {
	afterEach(async () => {
		vi.useRealTimers();
		vi.restoreAllMocks();
		await WS.clean();
	});

	it("runs an exited command again when asked", async () => {
		const { runAgain, nextWs, onEndedChange } =
			await renderEndedCommandTerminal();
		expect(onEndedChange).toHaveBeenLastCalledWith(true);

		await userEvent.click(runAgain);

		await nextWs.nextMessage;
		expect(nextWs.server.clients()[0].url).toContain("command=echo+hello");
		expect(onEndedChange).toHaveBeenLastCalledWith(false);
	});

	it("does not connect a tab whose command already ended until asked", async () => {
		const ws = createWorkspaceTerminalWebSocket();
		const onConnect = vi.fn();
		ws.on("connection", onConnect);
		renderWithAuth(<CommandTerminalTab initiallyEnded />);
		const runAgain = await screen.findByRole("button", { name: "Run again" });
		const user = setupUserWithFakeTimers();
		await vi.advanceTimersByTimeAsync(PAST_RECONNECT_WINDOW_MS);
		expect(onConnect).not.toHaveBeenCalled();

		await user.click(runAgain);
		await vi.advanceTimersByTimeAsync(PAST_RECONNECT_WINDOW_MS);

		expect(onConnect).toHaveBeenCalledOnce();
	});

	it("does not take focus when its command exits", async () => {
		const ws = createWorkspaceTerminalWebSocket();
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
		// The terminal takes focus in an animation frame, so wait one out
		// before checking that it did not.
		await new Promise(requestAnimationFrame);

		expect(elsewhere).toHaveFocus();
	});

	it("keeps an ended terminal and its output while the tab is hidden", async () => {
		const openTerminal = vi.spyOn(Terminal.prototype, "open");
		const { onReconnect } = await renderEndedCommandTerminal();
		const user = setupUserWithFakeTimers();
		const switchTab = screen.getByRole("button", { name: "Switch tab" });

		await user.click(switchTab);
		await vi.advanceTimersByTimeAsync(TERMINAL_IDLE_DETACH_MS + 1);
		await user.click(switchTab);
		await vi.advanceTimersByTimeAsync(PAST_RECONNECT_WINDOW_MS);

		expect(openTerminal).toHaveBeenCalledOnce();
		expect(onReconnect).not.toHaveBeenCalled();
	});

	it("does not run an exited command again when its connection settings change", async () => {
		const { onReconnect } = await renderEndedCommandTerminal();
		const user = setupUserWithFakeTimers();

		await user.click(screen.getByRole("button", { name: "Change agent" }));
		await vi.advanceTimersByTimeAsync(PAST_RECONNECT_WINDOW_MS);

		expect(onReconnect).not.toHaveBeenCalled();
	});
});
