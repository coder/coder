import { createBrowserRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { insertChatListHistoryEntry } from "./chatListHistoryEntry";

const chatPath = "/agents/8f3a6131-1ce8-46f5-9ba8-4a36beb2a1c4";

// jsdom keeps one session history per test file, so each test fakes the
// window it opens: its first entry, its history length and its display mode.
const openWindow = (
	path: string,
	{ standalone = true, historyLength = 1 } = {},
) => {
	window.history.replaceState(null, "", path);
	vi.spyOn(window.history, "length", "get").mockReturnValue(historyLength);
	vi.stubGlobal("matchMedia", (query: string) => ({
		matches: standalone && query === "(display-mode: standalone)",
		media: query,
	}));
};

const currentEntry = () => ({
	url: `${window.location.pathname}${window.location.search}${window.location.hash}`,
	state: window.history.state,
});

afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

describe("insertChatListHistoryEntry", () => {
	it("gives a standalone window opened on a chat a way back to the chat list", async () => {
		openWindow(`${chatPath}?archived=false#latest`);

		insertChatListHistoryEntry();
		const router = createBrowserRouter([
			{ path: "/agents", children: [{ index: true }, { path: ":agentId" }] },
		]);

		expect(router.state.location).toMatchObject({
			pathname: chatPath,
			search: "?archived=false",
			hash: "#latest",
		});
		expect(router.state.location.key).not.toBe("default");

		const backToList = new Promise<void>((resolve) => {
			const unsubscribe = router.subscribe((state) => {
				if (state.location.pathname === "/agents") {
					unsubscribe();
					resolve();
				}
			});
		});
		await router.navigate(-1);
		await backToList;
		// Matches a window opened on the chat list itself.
		expect(router.state.location).toMatchObject({
			pathname: "/agents",
			search: "?archived=false",
			key: "default",
		});
		router.dispose();
	});

	it.each([
		{ name: "a browser tab", path: chatPath, standalone: false },
		{ name: "a window with a back entry", path: chatPath, historyLength: 2 },
		{ name: "the chat list", path: "/agents" },
		{ name: "agent settings", path: "/agents/settings" },
		{ name: "an agent settings page", path: "/agents/settings/general" },
		{ name: "an embedded chat", path: `${chatPath}/embed` },
		{ name: "another page", path: "/workspaces" },
	])("leaves the history of $name alone", ({ path, ...options }) => {
		openWindow(path, options);
		const before = currentEntry();

		insertChatListHistoryEntry();

		expect(currentEntry()).toEqual(before);
	});
});
