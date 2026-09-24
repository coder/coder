import { QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import { MockWorkspace } from "#/testHelpers/entities";
import { boardChatsKey } from "./boardChats";
import { addCommentLabels, buildCards } from "./boardLabels";
import { assistantIds, openCardAssistant, snapshot } from "./cardAssistant";

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

/** Opens with mocked mutations and returns them for inspection. */
const open = (chats: readonly Chat[], existing: Chat | undefined) => {
	const create = vi.fn().mockResolvedValue({ ...MockChat, id: "created" });
	const rename = vi.fn().mockResolvedValue(undefined);
	const result = openCardAssistant({
		card: cardFor(chats),
		existingId: existing?.id,
		create,
		rename,
		queryClient: new QueryClient(),
	});
	return { result, create, rename };
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

describe("openCardAssistant", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		localStorage.clear();
	});

	it("finds the assistant chat by its card label", () => {
		const card = cardFor([chat("p")]);
		const helper = chat("h", { "board/assistant": "p" });
		expect(assistantIds([chat("p"), helper]).get(card.id)).toBe(helper.id);
		expect(assistantIds([chat("p")]).get(card.id)).toBeUndefined();
	});

	it("reuses an existing assistant without any request", async () => {
		const getWorkspaces = vi.spyOn(API, "getWorkspaces");
		const { result, create } = open(
			[chat("p")],
			chat("h", { "board/assistant": "p" }),
		);

		expect(await result).toBe("h");
		expect(getWorkspaces).not.toHaveBeenCalled();
		expect(create).not.toHaveBeenCalled();
	});

	it("creates the assistant in the shared workspace and titles it", async () => {
		vi.spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [{ ...MockWorkspace, id: "ws-1", name: "agents-kanban" }],
			count: 1,
		});

		const { result, create, rename } = open(
			[chat("p", { "board/title": "Epic" })],
			undefined,
		);

		expect(await result).toBe("created");
		expect(API.getWorkspaces).toHaveBeenCalledWith({
			q: "owner:me name:agents-kanban",
		});
		expect(create).toHaveBeenCalledTimes(1);
		const request = create.mock.calls[0]?.[0];
		expect(request).toMatchObject({
			organization_id: MockChat.organization_id,
			workspace_id: "ws-1",
			labels: { "board/assistant": "p" },
			client_type: "ui",
		});
		expect(request?.system_prompt).toContain("assistant for one card");
		expect(request?.content[0]).toMatchObject({ type: "text" });
		expect(rename).toHaveBeenCalledWith({
			chatId: "created",
			title: "Assistant: Epic",
		});
	});

	it("tells the snapshot when no workspace exists and omits workspace_id", async () => {
		vi.spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});

		const { result, create } = open([chat("p")], undefined);
		await result;

		const request = create.mock.calls[0]?.[0];
		expect(request?.workspace_id).toBeUndefined();
		expect(request?.model_config_id).toBeUndefined();
		const first = request?.content[0];
		expect(first?.type === "text" && first.text).toContain(
			"Workspace: none attached yet",
		);
	});

	it("toasts and returns undefined when the workspace lookup fails", async () => {
		const { toast } = await import("sonner");
		vi.spyOn(API, "getWorkspaces").mockRejectedValue(new Error("offline"));

		const { result } = open([chat("p")], undefined);

		expect(await result).toBeUndefined();
		expect(toast.error).toHaveBeenCalledWith("offline");
	});

	it("still opens a created chat when only the rename fails", async () => {
		vi.spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});
		const create = vi.fn().mockResolvedValue({ ...MockChat, id: "created" });
		const rename = vi.fn().mockRejectedValue(new Error("rename failed"));

		const id = await openCardAssistant({
			card: cardFor([chat("p")]),
			existingId: undefined,
			create,
			rename,
			queryClient: new QueryClient(),
		});

		expect(id).toBe("created");
		expect(create).toHaveBeenCalledTimes(1);
	});

	it("shares one creation between concurrent opens of the same card on one client", async () => {
		const lookup = createDeferred<{ workspaces: never[]; count: number }>();
		vi.spyOn(API, "getWorkspaces").mockReturnValue(lookup.promise);
		const queryClient = new QueryClient();
		const card = cardFor([chat("p")]);
		const create = vi.fn().mockResolvedValue({ ...MockChat, id: "created" });
		const rename = vi.fn().mockResolvedValue(undefined);
		const args = { card, existingId: undefined, create, rename, queryClient };

		const first = openCardAssistant(args);
		const second = openCardAssistant(args);
		const elsewhere = openCardAssistant({
			...args,
			queryClient: new QueryClient(),
		});
		lookup.resolve({ workspaces: [], count: 0 });

		expect(await Promise.all([first, second, elsewhere])).toEqual([
			"created",
			"created",
			"created",
		]);
		expect(create).toHaveBeenCalledTimes(2);
		expect(API.getWorkspaces).toHaveBeenCalledTimes(2);
	});

	it("prepends the created chat to the board list so the next open finds it", async () => {
		vi.spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});
		const queryClient = new QueryClient();
		const card = cardFor([chat("p")]);
		queryClient.setQueryData(boardChatsKey, {
			pages: [[chat("p")]],
			pageParams: [0],
		});
		const create = vi
			.fn()
			.mockResolvedValue(chat("created", { "board/assistant": card.id }));
		const args = {
			card,
			create,
			rename: vi.fn().mockResolvedValue(undefined),
			queryClient,
		};

		await openCardAssistant({ ...args, existingId: undefined });
		// What the page computes from the list on its next render.
		const listed =
			queryClient.getQueryData<{ pages: Chat[][] }>(boardChatsKey)?.pages[0] ??
			[];
		const existingId = assistantIds(listed).get(card.id);
		const again = await openCardAssistant({ ...args, existingId });

		expect(existingId).toBe("created");
		expect(again).toBe("created");
		expect(create).toHaveBeenCalledTimes(1);
	});
});
