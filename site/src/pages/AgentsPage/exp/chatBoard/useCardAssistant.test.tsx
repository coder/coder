import { renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockWorkspace } from "#/testHelpers/entities";
import { addCommentLabels, buildCards } from "./boardLabels";
import { findAssistant, snapshot, useCardAssistant } from "./useCardAssistant";

vi.mock("sonner", () => ({
	toast: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
}));

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	title: `Chat ${id}`,
	labels,
});

const cardFor = (chats: readonly Chat[]) => {
	const [card] = buildCards(chats);
	if (!card) throw new Error("card missing");
	return card;
};

const renderAssistant = (chats: readonly Chat[]) => {
	const queryClient = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	});
	const wrapper = ({ children }: PropsWithChildren) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	return renderHook(() => useCardAssistant(chats, buildCards(chats)), {
		wrapper,
	}).result;
};

describe("snapshot", () => {
	it("lists notes oldest first and numbers the chats", () => {
		const card = cardFor([
			chat("p", {
				"board/title": "Epic",
				"board/column": "Doing",
				...addCommentLabels(
					addCommentLabels({}, "later", 2000),
					"sooner",
					1000,
				),
			}),
			{
				...chat("m", { "board/group": "p" }),
				status: "running",
				summary: " done \n",
			},
		]);
		const text = snapshot(card, false);
		expect(text).toContain('Assistant for card "Epic"');
		expect(text).toContain("Column: Doing");
		expect(text.indexOf("sooner")).toBeLessThan(text.indexOf("later"));
		expect(text).toContain("1. Chat p\n   id: p\n   status: waiting");
		expect(text).toContain("2. Chat m\n   id: m\n   status: running");
		expect(text).toContain("summary: done");
		expect(text).not.toContain("Workspace: none attached yet");
	});

	it("adds the workspace instruction only when the workspace is missing", () => {
		const card = cardFor([chat("p")]);
		expect(snapshot(card, true)).toContain("Workspace: none attached yet");
		expect(snapshot(card, true)).toContain("Notes: none");
	});
});

describe("useCardAssistant", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		localStorage.clear();
	});

	it("finds the assistant chat by its card label", () => {
		const card = cardFor([chat("p")]);
		const helper = chat("h", { "board/assistant": "p" });
		expect(findAssistant(card, [chat("p"), helper])).toBe(helper);
		expect(findAssistant(card, [chat("p")])).toBeUndefined();
	});

	it("reuses an existing assistant without any request", async () => {
		const getWorkspaces = vi.spyOn(API, "getWorkspaces");
		const createChat = vi.spyOn(API.experimental, "createChat");
		const chats = [chat("p")];

		const id = await renderAssistant(chats).current.open(
			cardFor(chats),
			chat("h", { "board/assistant": "p" }),
		);

		expect(id).toBe("h");
		expect(getWorkspaces).not.toHaveBeenCalled();
		expect(createChat).not.toHaveBeenCalled();
	});

	it("creates the assistant in the shared workspace and titles it", async () => {
		vi.spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [{ ...MockWorkspace, id: "ws-1", name: "agents-kanban" }],
			count: 1,
		});
		const createChat = vi
			.spyOn(API.experimental, "createChat")
			.mockResolvedValue({ ...MockChat, id: "new-assistant" });
		const updateChat = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		localStorage.setItem("agents.last-model-config-id", "model-9");
		const chats = [chat("p", { "board/title": "Epic" })];

		const id = await renderAssistant(chats).current.open(
			cardFor(chats),
			undefined,
		);

		expect(id).toBe("new-assistant");
		expect(API.getWorkspaces).toHaveBeenCalledWith({
			q: "owner:me name:agents-kanban",
		});
		expect(createChat).toHaveBeenCalledTimes(1);
		const request = createChat.mock.calls[0]?.[0];
		expect(request).toMatchObject({
			organization_id: MockChat.organization_id,
			workspace_id: "ws-1",
			labels: { "board/assistant": "p" },
			client_type: "ui",
			model_config_id: "model-9",
		});
		expect(request?.system_prompt).toContain("assistant for one card");
		expect(request?.content[0]).toMatchObject({ type: "text" });
		expect(updateChat).toHaveBeenCalledWith("new-assistant", {
			title: "Assistant: Epic",
		});
	});

	it("tells the snapshot when no workspace exists and omits workspace_id", async () => {
		vi.spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});
		const createChat = vi
			.spyOn(API.experimental, "createChat")
			.mockResolvedValue({ ...MockChat, id: "n" });
		vi.spyOn(API.experimental, "updateChat").mockResolvedValue(undefined);
		const chats = [chat("p")];

		await renderAssistant(chats).current.open(cardFor(chats), undefined);

		const request = createChat.mock.calls[0]?.[0];
		expect(request?.workspace_id).toBeUndefined();
		expect(request?.model_config_id).toBeUndefined();
		const first = request?.content[0];
		expect(first?.type === "text" && first.text).toContain(
			"Workspace: none attached yet",
		);
	});

	it("toasts and returns undefined when creation fails", async () => {
		const { toast } = await import("sonner");
		vi.spyOn(API, "getWorkspaces").mockRejectedValue(new Error("offline"));
		const chats = [chat("p")];

		const id = await renderAssistant(chats).current.open(
			cardFor(chats),
			undefined,
		);

		expect(id).toBeUndefined();
		expect(toast.error).toHaveBeenCalledWith("offline");
	});
});
