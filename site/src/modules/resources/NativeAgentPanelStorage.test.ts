import { MockOpenCodeNativeAgentSessionInfo } from "#/testHelpers/nativeAgent";
import {
	nativeAgentPanelStorageKey,
	readNativeAgentPanelState,
	writeNativeAgentPanelState,
} from "./NativeAgentPanelStorage";

const mockState = {
	cwd: "/home/coder/project",
	harness: "opencode",
	prompt: "An unsent draft",
	selectedSession: {
		sessionId: MockOpenCodeNativeAgentSessionInfo.sessionId,
		cwd: MockOpenCodeNativeAgentSessionInfo.cwd,
	},
	collapsedSessionID: MockOpenCodeNativeAgentSessionInfo.sessionId,
};

beforeEach(() => sessionStorage.clear());
afterEach(() => vi.restoreAllMocks());

it("uses stable case-insensitive agent names and unambiguous scoped keys", () => {
	expect(nativeAgentPanelStorageKey("user", "workspace", "MAIN")).toBe(
		nativeAgentPanelStorageKey("user", "workspace", "main"),
	);
	const scopes = [
		{ user: "user", workspace: "workspace", agent: "main" },
		{ user: "other", workspace: "workspace", agent: "main" },
		{ user: "user", workspace: "other", agent: "main" },
		{ user: "user", workspace: "workspace", agent: "other" },
		{ user: "user:workspace", workspace: "other", agent: "main" },
		{ user: "user", workspace: "workspace:other", agent: "main" },
	];
	expect(
		new Set(
			scopes.map(({ user, workspace, agent }) =>
				nativeAgentPanelStorageKey(user, workspace, agent),
			),
		).size,
	).toBe(scopes.length);
});

it("round-trips only UI state", () => {
	writeNativeAgentPanelState("panel", mockState);
	expect(readNativeAgentPanelState("panel")).toEqual(mockState);
});

it.each([
	"not json",
	"null",
	"[]",
	"{}",
	JSON.stringify({ ...mockState, prompt: 123 }),
	JSON.stringify({ ...mockState, prompt: "x".repeat(32_001) }),
	JSON.stringify({
		...mockState,
		selectedSession: { sessionId: 123, cwd: "/" },
	}),
	JSON.stringify({
		...mockState,
		selectedSession: {
			...mockState.selectedSession,
			_meta: { token: "not persisted" },
		},
	}),
	JSON.stringify({ ...mockState, transcript: "not persisted" }),
	" ".repeat(256_001),
])("ignores malformed, oversized, or unexpected stored fields: %#", (raw) => {
	sessionStorage.setItem("panel", raw);
	expect(readNativeAgentPanelState("panel")).toBeUndefined();
});

it("does not revive an older saved draft when the new draft exceeds the bound", () => {
	writeNativeAgentPanelState("panel", mockState);
	const mockOversizedState = { ...mockState, prompt: "x".repeat(32_001) };
	writeNativeAgentPanelState("panel", mockOversizedState);
	expect(sessionStorage.getItem("panel")).toBeNull();
	expect(mockOversizedState.prompt).toHaveLength(32_001);
});

it("tolerates denied reads and writes without modifying the caller's draft", () => {
	vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
		throw new DOMException("Storage denied", "SecurityError");
	});
	vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
		throw new DOMException("Storage full", "QuotaExceededError");
	});
	expect(readNativeAgentPanelState("panel")).toBeUndefined();
	expect(() => writeNativeAgentPanelState("panel", mockState)).not.toThrow();
	expect(mockState.prompt).toBe("An unsent draft");
});
