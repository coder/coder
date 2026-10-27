import { render } from "@testing-library/react";
import {
	createBrowserRouter,
	NavigationType,
	Outlet,
	RouterProvider,
} from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	openChatListFirst,
	useOpenNotificationChat,
} from "./notificationWindow";

const chatPath = "/agents/8f3a6131-1ce8-46f5-9ba8-4a36beb2a1c4";

// jsdom keeps one session history per test file, so each test fakes the
// window it opens: its first entry, its history length, its display mode and
// the user metadata the server rendered.
const openWindow = (
	path: string,
	{
		standalone = true,
		historyLength = 1,
		userMetadata,
	}: {
		standalone?: boolean;
		historyLength?: number;
		userMetadata?: string;
	} = {},
) => {
	if (userMetadata !== undefined) {
		const meta = document.createElement("meta");
		meta.setAttribute("property", "user");
		meta.setAttribute("content", userMetadata);
		document.head.appendChild(meta);
	}
	window.history.replaceState(null, "", path);
	vi.spyOn(window.history, "length", "get").mockReturnValue(historyLength);
	vi.stubGlobal("matchMedia", (query: string) => ({
		matches: standalone && query === "(display-mode: standalone)",
		media: query,
	}));
};

const ChatsLayout = ({ isListLoading }: { isListLoading: boolean }) => {
	useOpenNotificationChat(isListLoading);
	return <Outlet />;
};

const createAgentsRouter = (isListLoading = false) =>
	createBrowserRouter([
		{
			path: "/agents",
			element: <ChatsLayout isListLoading={isListLoading} />,
			children: [{ index: true }, { path: "settings" }, { path: ":agentId" }],
		},
	]);

const currentEntry = () => ({
	url: `${window.location.pathname}${window.location.search}${window.location.hash}`,
	state: window.history.state,
});

afterEach(() => {
	for (const meta of document.head.querySelectorAll('meta[property="user"]')) {
		meta.remove();
	}
	vi.useRealTimers();
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

describe("notification window", () => {
	it("opens the chat on top of the chat list once the list has loaded", async () => {
		vi.useFakeTimers({
			toFake: [
				"setTimeout",
				"clearTimeout",
				"requestAnimationFrame",
				"cancelAnimationFrame",
			],
		});
		openWindow(`${chatPath}?archived=false#latest`);

		openChatListFirst();
		const router = createAgentsRouter();
		render(<RouterProvider router={router} />);

		expect(router.state.location).toMatchObject({
			pathname: "/agents",
			search: "?archived=false",
		});
		await vi.advanceTimersByTimeAsync(50);
		expect(router.state.location).toMatchObject({
			pathname: chatPath,
			search: "?archived=false",
			hash: "#latest",
		});
		expect(router.state.historyAction).toBe(NavigationType.Push);
		router.dispose();
	});

	it("opens the chat after 1.5 s while the list is still loading", async () => {
		vi.useFakeTimers({
			toFake: [
				"setTimeout",
				"clearTimeout",
				"requestAnimationFrame",
				"cancelAnimationFrame",
			],
		});
		openWindow(chatPath);

		openChatListFirst();
		const router = createAgentsRouter(true);
		render(<RouterProvider router={router} />);

		await vi.advanceTimersByTimeAsync(1400);
		expect(router.state.location.pathname).toBe("/agents");
		await vi.advanceTimersByTimeAsync(200);
		expect(router.state.location.pathname).toBe(chatPath);
		router.dispose();
	});

	it("does not open the chat once the user has left the list", async () => {
		vi.useFakeTimers({
			toFake: [
				"setTimeout",
				"clearTimeout",
				"requestAnimationFrame",
				"cancelAnimationFrame",
			],
		});
		openWindow(chatPath);

		openChatListFirst();
		const router = createAgentsRouter(true);
		render(<RouterProvider router={router} />);
		await router.navigate("/agents/settings");

		await vi.advanceTimersByTimeAsync(2000);
		expect(router.state.location.pathname).toBe("/agents/settings");
		router.dispose();
	});

	it.each([
		{ name: "a browser tab", path: chatPath, standalone: false },
		{ name: "a window with a back entry", path: chatPath, historyLength: 2 },
		{ name: "a signed-out visitor", path: chatPath, userMetadata: "" },
		{ name: "the chat list", path: "/agents" },
		{ name: "agent settings", path: "/agents/settings" },
		{ name: "an agent settings page", path: "/agents/settings/general" },
		{ name: "an embedded chat", path: `${chatPath}/embed` },
		{ name: "another page", path: "/workspaces" },
	])("leaves the history of $name alone", ({ path, ...options }) => {
		openWindow(path, options);
		const before = currentEntry();

		openChatListFirst();

		expect(currentEntry()).toEqual(before);
	});
});
