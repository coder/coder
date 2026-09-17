import { QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockWorkspace } from "#/testHelpers/entities";
import { cardAssistantSpec } from "./assistantSpecs";
import { assistantIds, openAssistant } from "./assistants";
import { buildCards } from "./boardLabels";

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

/** Opens the card assistant with mocked mutations and returns them for inspection. */
const open = (chats: readonly Chat[], existing: Chat | undefined) => {
	const create = vi.fn().mockResolvedValue({ ...MockChat, id: "created" });
	const rename = vi.fn().mockResolvedValue(undefined);
	const result = openAssistant({
		spec: cardAssistantSpec(cardFor(chats)),
		existingId: existing?.id,
		create,
		rename,
		queryClient: new QueryClient(),
	});
	return { result, create, rename };
};

describe("assistantIds", () => {
	it("maps every assistant label value to its chat", () => {
		const helper = chat("h", { "board/assistant": "p" });
		const board = chat("b", { "board/assistant": "board" });
		const ids = assistantIds([chat("p"), helper, board]);
		expect(ids.get("p")).toBe("h");
		expect(ids.get("board")).toBe("b");
		expect(assistantIds([chat("p")]).get("p")).toBeUndefined();
	});
});

describe("openAssistant", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		localStorage.clear();
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
		localStorage.setItem("agents.last-model-config-id", "model-9");

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
			model_config_id: "model-9",
		});
		expect(request?.system_prompt).toContain("assistant for one card");
		const first = request?.content[0];
		expect(first?.type === "text" && first.text).not.toContain(
			"Workspace: none attached yet",
		);
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

	it("toasts and returns undefined when creation fails", async () => {
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

		const id = await openAssistant({
			spec: cardAssistantSpec(cardFor([chat("p")])),
			existingId: undefined,
			create,
			rename,
			queryClient: new QueryClient(),
		});

		expect(id).toBe("created");
		expect(create).toHaveBeenCalledTimes(1);
	});
});
