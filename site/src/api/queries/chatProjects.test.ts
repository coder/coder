import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockChatProject } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	chatProject,
	chatProjectsKey,
	createChatProject,
	deleteChatProject,
	updateChatProject,
} from "./chatProjects";
import {
	chatEntitiesFamilyKey,
	chatEntityKey,
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
	// The callbacks ignore their arguments.
	const settle = (options: { onSettled?: (...args: never[]) => unknown }) =>
		options.onSettled?.();
	const isInvalidated = (
		queryClient: ReturnType<typeof createTestQueryClient>,
		key: readonly unknown[],
	) => queryClient.getQueryState(key)?.isInvalidated;

	it("refreshes only the project list after a create or update", async () => {
		for (const factory of [createChatProject, updateChatProject]) {
			const queryClient = seed();
			await settle(factory(queryClient));
			expect(isInvalidated(queryClient, chatProjectsKey)).toBe(true);
			for (const key of chatKeys) {
				expect(isInvalidated(queryClient, key)).toBe(false);
			}
		}
	});

	it("refreshes chat lists and searches after a delete", async () => {
		const queryClient = seed();
		const messagesKey = chatMessagesKey("chat-1");
		queryClient.setQueryData(messagesKey, {});
		await settle(deleteChatProject(queryClient));
		const [listKey, searchKey] = chatKeys;
		for (const key of [...projectKeys, listKey, searchKey]) {
			expect(isInvalidated(queryClient, key)).toBe(true);
		}
		expect(isInvalidated(queryClient, messagesKey)).toBe(false);
	});

	it("resets only the deleted project's chats once the delete succeeds", async () => {
		const queryClient = createTestQueryClient();
		const root = { ...MockChat, id: "root", project_id: MockChatProject.id };
		const child = { ...MockChat, id: "child", root_chat_id: root.id };
		const other = { ...MockChat, id: "other", project_id: "other-project" };
		for (const chat of [root, child, other]) {
			queryClient.setQueryData(chatEntityKey(chat.id), chat);
		}
		vi.spyOn(API.experimental, "deleteChatProject").mockResolvedValue();
		await deleteChatProject(queryClient).mutationFn?.(MockChatProject);
		expect(queryClient.getQueryData(chatEntityKey(root.id))).toBeUndefined();
		expect(queryClient.getQueryData(chatEntityKey(child.id))).toBeUndefined();
		expect(queryClient.getQueryData(chatEntityKey(other.id))).toEqual(other);
	});

	it("keeps cached chat entities when the delete fails", async () => {
		const queryClient = createTestQueryClient();
		const root = { ...MockChat, id: "root", project_id: MockChatProject.id };
		queryClient.setQueryDefaults(chatEntityKey(root.id), {
			gcTime: Number.POSITIVE_INFINITY,
		});
		queryClient.setQueryData(chatEntityKey(root.id), root);
		vi.spyOn(API.experimental, "deleteChatProject").mockRejectedValue(
			new Error("deadlock"),
		);
		await expect(
			deleteChatProject(queryClient).mutationFn?.(MockChatProject),
		).rejects.toThrow("deadlock");
		expect(queryClient.getQueryData(chatEntityKey(root.id))).toEqual(root);
	});
});
