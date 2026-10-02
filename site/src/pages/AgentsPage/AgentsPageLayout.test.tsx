import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockWorkspace,
	MockWorkspaceBuildDelete,
} from "#/testHelpers/entities";
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
});

describe("AgentsPageLayout archive and delete", () => {
	it("deletes two workspaces without waiting for the first", async () => {
		const user = userEvent.setup({ delay: null });
		const createdAt = "2024-06-01T00:00:00.000Z";
		vi.spyOn(API.experimental, "getChats").mockResolvedValue([
			{
				...MockChat,
				id: "chat-alpha",
				title: "Alpha agent",
				workspace_id: "ws-alpha",
				created_at: createdAt,
			},
			{
				...MockChat,
				id: "chat-beta",
				title: "Beta agent",
				workspace_id: "ws-beta",
				created_at: createdAt,
			},
		]);
		vi.spyOn(API.experimental, "updateChat").mockResolvedValue(undefined);
		vi.spyOn(API, "getWorkspace").mockImplementation(async (workspaceId) => ({
			...MockWorkspace,
			id: workspaceId,
			created_at: createdAt,
		}));
		vi.spyOn(API, "getWorkspaceBuilds").mockResolvedValue([]);
		let releaseDelete = () => {};
		const pendingDelete = new Promise<typeof MockWorkspaceBuildDelete>(
			(resolve) => {
				releaseDelete = () => resolve(MockWorkspaceBuildDelete);
			},
		);
		vi.spyOn(API, "deleteWorkspace").mockImplementation(() => pendingDelete);

		try {
			renderLayout();
			await user.click(
				await screen.findByRole("button", {
					name: "Open actions for Alpha agent",
				}),
			);
			await user.click(
				await screen.findByRole("menuitem", {
					name: "Archive & delete workspace",
				}),
			);
			await waitFor(
				() => {
					expect(API.deleteWorkspace).toHaveBeenCalledWith("ws-alpha");
				},
				{ timeout: 4000 },
			);
			await user.click(
				await screen.findByRole(
					"button",
					{ name: "Open actions for Beta agent" },
					{ timeout: 4000 },
				),
			);
			await user.click(
				await screen.findByRole("menuitem", {
					name: "Archive & delete workspace",
				}),
			);
			await waitFor(
				() => {
					expect(API.deleteWorkspace).toHaveBeenCalledWith("ws-beta");
				},
				{ timeout: 4000 },
			);
		} finally {
			releaseDelete();
		}
	});
});
