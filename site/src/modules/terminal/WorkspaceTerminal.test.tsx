import { act } from "@testing-library/react";
import { Terminal } from "@xterm/xterm";
import { createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import {
	createWorkspaceTerminalWebSocket,
	normalClosure,
	PAST_RECONNECT_WINDOW_MS,
	setupUserWithFakeTimers,
	WS,
} from "./testHelpers";
import {
	WorkspaceTerminal,
	type WorkspaceTerminalHandle,
} from "./WorkspaceTerminal";

type CommandTerminalProps = Partial<
	React.ComponentProps<typeof WorkspaceTerminal>
>;

const CommandTerminal: React.FC<CommandTerminalProps> = (props) => {
	return (
		<WorkspaceTerminal
			agentId={MockWorkspaceAgent.id}
			reconnectionToken="first-session"
			sessionId="0123456789abcdef0123456789abcdef"
			initialCommand="echo hello"
			{...props}
		/>
	);
};

// Fake timers go in before the close so the websocket-ts retry and the mock
// server's accept wait for the test.
const renderEndedCommandTerminal = async (props: CommandTerminalProps = {}) => {
	const ws = createWorkspaceTerminalWebSocket();
	const view = renderComponent(<CommandTerminal {...props} />);
	await ws.nextMessage;
	setupUserWithFakeTimers();
	ws.close(normalClosure);
	const onReconnect = vi.fn();
	createWorkspaceTerminalWebSocket().on("connection", onReconnect);
	return { ...view, onReconnect };
};

describe("WorkspaceTerminal", () => {
	afterEach(async () => {
		vi.useRealTimers();
		vi.restoreAllMocks();
		await WS.clean();
	});

	it("reports initializing as soon as it is asked to run again", async () => {
		const ref = createRef<WorkspaceTerminalHandle>();
		const onStatusChange = vi.fn();
		await renderEndedCommandTerminal({ ref, onStatusChange });
		expect(onStatusChange).toHaveBeenLastCalledWith("ended");

		act(() => ref.current?.runAgain());

		expect(onStatusChange).toHaveBeenLastCalledWith("initializing");
	});

	it("connects a new session after the previous one ended", async () => {
		const { rerender, onReconnect } = await renderEndedCommandTerminal();

		rerender(<CommandTerminal reconnectionToken="second-session" />);
		await vi.advanceTimersByTimeAsync(PAST_RECONNECT_WINDOW_MS);

		expect(onReconnect).toHaveBeenCalledOnce();
	});

	it("does not connect a terminal created after the session ended", async () => {
		const openTerminal = vi.spyOn(Terminal.prototype, "open");
		const { rerender, onReconnect } = await renderEndedCommandTerminal();

		rerender(<CommandTerminal terminalFontFamily="serif" />);
		await vi.advanceTimersByTimeAsync(PAST_RECONNECT_WINDOW_MS);

		expect(openTerminal).toHaveBeenCalledTimes(2);
		expect(onReconnect).not.toHaveBeenCalled();
	});
});
