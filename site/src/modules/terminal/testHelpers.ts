import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { MockWorkspaceAgent } from "#/testHelpers/entities";

vi.stubGlobal("jest", vi);
await import("jest-canvas-mock");
const { default: WS } = await import("jest-websocket-mock");

export { WS };

/**
 * Starts a mock PTY server for `MockWorkspaceAgent`. The mock server matches
 * URLs without the query string.
 */
export const createWorkspaceTerminalWebSocket = () => {
	return new WS(
		`ws://${location.host}/api/v2/workspaceagents/${MockWorkspaceAgent.id}/pty`,
	);
};

/** The close coderd sends once the agent ends the session. */
export const normalClosure = { code: 1000, reason: "", wasClean: true };

/** Long enough for websocket-ts to have retried after a close. */
export const PAST_RECONNECT_WINDOW_MS = 60_000;

/**
 * Puts timers under test control, because websocket-ts retries and the mock
 * server accepts connections on timers. Returns a user that advances them.
 */
export const setupUserWithFakeTimers = () => {
	vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
	return userEvent.setup({ advanceTimers: vi.advanceTimersByTimeAsync });
};
