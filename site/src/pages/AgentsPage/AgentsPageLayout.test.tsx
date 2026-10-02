import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useOutletContext } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockChat } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockWorkspace,
	MockWorkspaceBuildDelete,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import AgentsPageLayout, {
	type AgentsPageOutletContext,
} from "./AgentsPageLayout";
import { emptyInputStorageKey } from "./components/AgentCreateForm";
import { ChatTopBar } from "./components/ChatTopBar";

const renderLayout = (route = "/agents") =>
	renderWithAuth(<AgentsPageLayout />, {
		path: "/agents",
		route,
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

describe("AgentsPageLayout archive state", () => {
	it.each([
		{ surface: "sidebar", succeeds: true },
		{ surface: "top bar", succeeds: true },
		{ surface: "sidebar", succeeds: false },
		{ surface: "top bar", succeeds: false },
	])(
		"$surface clears chat errors only on archive success ($succeeds)",
		async ({ surface, succeeds }) => {
			const user = userEvent.setup();
			const error = { message: "Agent failed", kind: "generic" as const };
			const readError = vi.fn();
			const ChatRoute = () => {
				const { chatErrorReasons, setChatErrorReason } =
					useOutletContext<AgentsPageOutletContext>();
				return (
					<>
						<button
							type="button"
							onClick={() => setChatErrorReason(MockChat.id, error)}
						>
							Set error
						</button>
						<button
							type="button"
							onClick={() => readError(chatErrorReasons[MockChat.id])}
						>
							Read error
						</button>
						<ChatTopBar
							chat={MockChat}
							panel={{ showSidebarPanel: false, onToggleSidebar: vi.fn() }}
						/>
					</>
				);
			};
			vi.spyOn(API.experimental, "getChats").mockResolvedValue([MockChat]);
			const update = vi.spyOn(API.experimental, "updateChat");
			if (succeeds) {
				update.mockResolvedValue(undefined);
			} else {
				update.mockRejectedValue(new Error("Archive rejected"));
			}
			const { queryClient } = renderWithAuth(<AgentsPageLayout />, {
				path: "/agents",
				route: `/agents/${MockChat.id}`,
				children: [{ path: ":agentId", element: <ChatRoute /> }],
			});
			await user.click(
				await screen.findByRole("button", { name: "Set error" }),
			);
			await user.click(screen.getByRole("button", { name: "Read error" }));
			expect(readError).toHaveBeenLastCalledWith(error);
			await user.click(
				await screen.findByRole("button", {
					name:
						surface === "sidebar"
							? `Open actions for ${MockChat.title}`
							: "Open agent actions",
				}),
			);
			await user.click(
				await screen.findByRole("menuitem", { name: "Archive agent" }),
			);
			await waitFor(() => {
				expect(update).toHaveBeenCalledWith(MockChat.id, { archived: true });
				expect(queryClient.isMutating()).toBe(0);
			});
			await user.click(screen.getByRole("button", { name: "Read error" }));
			expect(readError).toHaveBeenLastCalledWith(succeeds ? undefined : error);
		},
	);

	it.each([
		{ action: "Archive agent", archived: false },
		{ action: "Unarchive agent", archived: true },
	])("$action does not block another chat", async ({ action, archived }) => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChats").mockResolvedValue([
			{ ...MockChat, id: "chat-alpha", title: "Alpha agent", archived },
			{ ...MockChat, id: "chat-beta", title: "Beta agent", archived },
		]);
		const pendingUpdate = createDeferred<undefined>();
		vi.spyOn(API.experimental, "updateChat").mockReturnValue(
			pendingUpdate.promise,
		);
		const { queryClient } = renderLayout(
			archived ? "/agents?archived=archived" : "/agents",
		);

		try {
			for (const [id, title] of [
				["chat-alpha", "Alpha agent"],
				["chat-beta", "Beta agent"],
			]) {
				await user.click(
					await screen.findByRole("button", {
						name: `Open actions for ${title}`,
					}),
				);
				await user.click(await screen.findByRole("menuitem", { name: action }));
				await waitFor(() => {
					expect(API.experimental.updateChat).toHaveBeenCalledWith(id, {
						archived: !archived,
					});
				});
			}
		} finally {
			pendingUpdate.resolve(undefined);
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		}
	});
});

describe("AgentsPageLayout archive and delete", () => {
	it.each(["sidebar", "top bar"])(
		"shares pending archive state from the %s with the other surface",
		async (surface) => {
			const user = userEvent.setup();
			const chat = {
				...MockChat,
				title: "Archive target",
				workspace_id: MockWorkspace.id,
			};
			vi.spyOn(API.experimental, "getChats").mockResolvedValue([chat]);
			vi.spyOn(API, "getWorkspace").mockResolvedValue({
				...MockWorkspace,
				created_at: chat.created_at,
			});
			vi.spyOn(API, "getWorkspaceBuilds").mockResolvedValue([]);
			vi.spyOn(API, "deleteWorkspace").mockResolvedValue(
				MockWorkspaceBuildDelete,
			);
			const pendingUpdate = createDeferred<undefined>();
			vi.spyOn(API.experimental, "updateChat").mockReturnValue(
				pendingUpdate.promise,
			);
			const { queryClient } = renderWithAuth(<AgentsPageLayout />, {
				path: "/agents",
				route: `/agents/${chat.id}`,
				children: [
					{
						path: ":agentId",
						element: (
							<ChatTopBar
								chat={chat}
								panel={{ showSidebarPanel: false, onToggleSidebar: vi.fn() }}
							/>
						),
					},
				],
			});
			const sidebarTrigger = `Open actions for ${chat.title}`;
			const topBarTrigger = "Open agent actions";
			try {
				await user.click(
					await screen.findByRole("button", {
						name: surface === "sidebar" ? sidebarTrigger : topBarTrigger,
					}),
				);
				await user.click(
					await screen.findByRole("menuitem", {
						name: "Archive & delete workspace",
					}),
				);
				await waitFor(() =>
					expect(API.experimental.updateChat).toHaveBeenCalledTimes(1),
				);
				if (surface === "sidebar") {
					await user.click(
						await screen.findByRole("button", { name: topBarTrigger }),
					);
				} else {
					await user.pointer({
						target: await screen.findByRole("link", {
							name: /^Archive target/,
						}),
						keys: "[MouseRight]",
					});
				}
				await user.click(
					await screen.findByRole("menuitem", { name: "Archive agent" }),
				);
				expect(API.experimental.updateChat).toHaveBeenCalledTimes(1);
				await user.keyboard("{Escape}");
			} finally {
				pendingUpdate.resolve(undefined);
				await waitFor(() => expect(queryClient.isMutating()).toBe(0));
			}
		},
	);

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
