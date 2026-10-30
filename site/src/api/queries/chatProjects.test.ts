import { MutationObserver } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProjectTrigger } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockChatProjectSlackTrigger,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	chatProject,
	chatProjectMCPServerTokensKey,
	chatProjectsKey,
	chatProjectTriggersKey,
	createChatProject,
	deleteChatProject,
	updateChatProject,
	updateChatProjectTrigger,
} from "./chatProjects";
import {
	chatEntitiesFamilyKey,
	chatListFamilyKey,
	chatMessagesKey,
	chatSearchFamilyKey,
} from "./chats";

describe("chatProject", () => {
	it("selects the project with the ID, or null when the list lacks it", () => {
		const other = { ...MockChatProject, id: "other-project" };
		const projects = [MockChatProject, other];

		expect(chatProject(other.id).select?.(projects)).toBe(other);
		expect(chatProject("missing").select?.(projects)).toBeNull();
	});
});

describe("chat project mutations", () => {
	const projectKeys = [chatProjectsKey];
	const chatKeys = [
		[...chatListFamilyKey, { q: "" }],
		[...chatSearchFamilyKey, { q: "x" }],
		[...chatEntitiesFamilyKey, "chat-1"],
	];
	const seed = () => {
		const queryClient = createTestQueryClient();
		for (const key of [...projectKeys, ...chatKeys]) {
			queryClient.setQueryData(key, {});
		}
		return queryClient;
	};
	// Calls onSettled as react-query does: with the data, the error, and the
	// variables the mutation ran with.
	const settle = (
		options: { onSettled?: (...args: never[]) => unknown },
		variables?: unknown,
	) =>
		options.onSettled?.(
			undefined as never,
			null as never,
			variables as never,
			undefined as never,
		);
	const isInvalidated = (
		queryClient: ReturnType<typeof createTestQueryClient>,
		key: readonly unknown[],
	) => queryClient.getQueryState(key)?.isInvalidated;

	it("refreshes only the project list after a create or update", async () => {
		const settles = [
			(queryClient: ReturnType<typeof seed>) =>
				settle(createChatProject(queryClient)),
			(queryClient: ReturnType<typeof seed>) =>
				settle(updateChatProject(queryClient), {
					project: MockChatProject,
					request: { name: "Renamed" },
				}),
		];
		for (const run of settles) {
			const queryClient = seed();
			await run(queryClient);
			expect(isInvalidated(queryClient, chatProjectsKey)).toBe(true);
			for (const key of chatKeys) {
				expect(isInvalidated(queryClient, key)).toBe(false);
			}
		}
	});

	it("refreshes the agent's MCP server tokens after its servers change", async () => {
		const queryClient = seed();
		const tokensKey = chatProjectMCPServerTokensKey(MockChatProject.id);
		queryClient.setQueryData(tokensKey, []);
		await settle(updateChatProject(queryClient), {
			project: MockChatProject,
			request: { mcp_server_ids: ["mcp-1"] },
		});
		expect(isInvalidated(queryClient, tokensKey)).toBe(true);
	});

	it("refreshes chat lists, searches, and entities after a delete", async () => {
		const queryClient = seed();
		const messagesKey = chatMessagesKey("chat-1");
		queryClient.setQueryData(messagesKey, {});
		await settle(deleteChatProject(queryClient));
		for (const key of [...projectKeys, ...chatKeys]) {
			expect(isInvalidated(queryClient, key)).toBe(true);
		}
		// A chat's nested queries do not depend on its project.
		expect(isInvalidated(queryClient, messagesKey)).toBe(false);
	});
});

describe("updateChatProjectTrigger", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("keeps new secrets out of the cached trigger list", async () => {
		const queryClient = createTestQueryClient();
		const key = chatProjectTriggersKey(MockChatProject.id);
		// Keep the list without a component observing it.
		queryClient.setQueryDefaults(key, { gcTime: Number.POSITIVE_INFINITY });
		const mockUnverifiedSlackTrigger = {
			...MockChatProjectSlackTrigger,
			has_slack_signing_secret: false,
		};
		queryClient.setQueryData<ChatProjectTrigger[]>(key, [
			mockUnverifiedSlackTrigger,
		]);
		vi.spyOn(API.experimental, "updateChatProjectTrigger").mockReturnValue(
			new Promise(() => {}),
		);

		void new MutationObserver(
			queryClient,
			updateChatProjectTrigger(queryClient, MockChatProject),
		).mutate({
			triggerId: mockUnverifiedSlackTrigger.id,
			request: {
				slack_signing_secret: "abc123",
				github_webhook_secret: "def456",
			},
		});

		await vi.waitFor(() =>
			expect(
				queryClient.getQueryData<ChatProjectTrigger[]>(key)?.[0],
			).toMatchObject({
				has_slack_signing_secret: true,
				has_github_webhook_secret: true,
			}),
		);
		const cached = JSON.stringify(queryClient.getQueryData(key));
		expect(cached).not.toContain("abc123");
		expect(cached).not.toContain("def456");
	});
});
