import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import AgentsPageLayout from "./AgentsPageLayout";
import { emptyInputStorageKey } from "./components/AgentCreateForm";

const renderLayout = () =>
	renderWithAuth(<AgentsPageLayout />, {
		path: "/agents",
		route: "/agents",
		children: [{ index: true, element: null }],
	});

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("AgentsPageLayout New chat", () => {
	// AgentCreatePage leaves a deep link's value in history state.
	it.each([
		["prompt", { prompt: "hi" }],
		["debug", { debugWorkspaceBuildId: "build-id" }],
	])(
		"keeps the saved draft when leaving a composer prefilled from a %s link",
		async (_, linkState) => {
			vi.spyOn(API.experimental, "getChats").mockResolvedValue([]);
			localStorage.setItem(
				emptyInputStorageKey,
				"draft the user typed earlier",
			);
			const user = userEvent.setup();

			const { router } = renderLayout();
			await router.navigate("/agents", { state: linkState });
			await user.click(await screen.findByRole("link", { name: "New chat" }));

			await waitFor(() => expect(router.state.location.state).toBeNull());
			expect(localStorage.getItem(emptyInputStorageKey)).toBe(
				"draft the user typed earlier",
			);
		},
	);
});
