import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import AgentsPageLayout from "./AgentsPageLayout";
import { emptyInputStorageKey } from "./components/AgentCreateForm";

const renderLayout = (route = "/agents") =>
	renderWithAuth(<AgentsPageLayout />, {
		path: "/agents",
		route,
		children: [
			{ index: true, element: null },
			{ path: "projects/:projectId", element: null },
			{ path: "board", element: null },
			{ path: ":agentId", element: null },
		],
	});

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("AgentsPageLayout New chat", () => {
	// A prompt link always replaces the draft in the composer, so New chat
	// keeps it. A debug link can fall back to the draft-backed composer, so
	// New chat clears the draft as it does on a plain composer.
	it.each([
		["prompt", { prompt: "hi" }, "draft the user typed earlier"],
		["debug", { debugWorkspaceBuildId: "build-id" }, null],
	])(
		"handles the saved draft on New chat after a %s link",
		async (_, linkState, expectedDraft) => {
			vi.spyOn(API.experimental, "getChats").mockResolvedValue([]);
			localStorage.setItem(
				emptyInputStorageKey,
				"draft the user typed earlier",
			);
			const user = userEvent.setup();

			const { router } = renderLayout();
			// AgentCreatePage leaves a deep link's value in history state.
			await router.navigate("/agents", { state: linkState });
			await user.click(await screen.findByRole("link", { name: "New chat" }));

			await waitFor(() => expect(router.state.location.state).toBeNull());
			expect(localStorage.getItem(emptyInputStorageKey)).toBe(expectedDraft);
		},
	);

	it.each([
		"/agents/chat-1",
		"/agents/projects/project-1",
		"/agents/board",
	])("keeps the plain composer's draft when leaving %s", async (route) => {
		vi.spyOn(API.experimental, "getChats").mockResolvedValue([]);
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");
		const user = userEvent.setup();

		const { router } = renderLayout(route);
		await user.click(await screen.findByRole("link", { name: "New chat" }));

		await waitFor(() => expect(router.state.location.pathname).toBe("/agents"));
		expect(localStorage.getItem(emptyInputStorageKey)).toBe(
			"draft the user typed earlier",
		);
	});
});
