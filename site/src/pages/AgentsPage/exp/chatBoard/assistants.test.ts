import { QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat } from "#/api/typesGenerated";
import { MockChat, MockMCPServerConfig } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import { MockWorkspace } from "#/testHelpers/entities";
import { type AssistantTools, cardAssistantSpec } from "./assistantSpecs";
import { assistantIds, openAssistant } from "./assistants";
import { boardChatsKey } from "./boardChats";
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

const coderMcp = {
	...MockMCPServerConfig,
	id: "mcp-coder",
	slug: "coder",
	url: "https://dev.coder.com/api/experimental/mcp/http",
	auth_type: "oauth2",
	enabled: true,
	auth_connected: true,
};

/** Opens the card assistant with mocked mutations and returns them for inspection. */
const open = (
	chats: readonly Chat[],
	existing: Chat | undefined,
	rename = vi.fn().mockResolvedValue(undefined),
) => {
	const create = vi.fn().mockResolvedValue({ ...MockChat, id: "created" });
	const result = openAssistant({
		spec: (tools) => cardAssistantSpec(cardFor(chats), tools),
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
		const getServers = vi.spyOn(API.experimental, "getMCPServerConfigs");
		const { result, create } = open(
			[chat("p")],
			chat("h", { "board/assistant": "p" }),
		);

		expect(await result).toBe("h");
		expect(getWorkspaces).not.toHaveBeenCalled();
		expect(getServers).not.toHaveBeenCalled();
		expect(create).not.toHaveBeenCalled();
	});

	it("creates the assistant in the shared workspace and titles it", async () => {
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
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
		expect(request?.mcp_server_ids).toBeUndefined();
		expect(request?.system_prompt).toContain("assistant for one card");
		expect(request?.system_prompt).not.toContain("coder_");
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
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
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
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		vi.spyOn(API, "getWorkspaces").mockRejectedValue(new Error("offline"));

		const { result } = open([chat("p")], undefined);

		expect(await result).toBeUndefined();
		expect(toast.error).toHaveBeenCalledWith("offline");
	});

	it("still opens a created chat when only the rename fails", async () => {
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		vi.spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});

		const { result, create } = open(
			[chat("p")],
			undefined,
			vi.fn().mockRejectedValue(new Error("rename failed")),
		);

		expect(await result).toBe("created");
		expect(create).toHaveBeenCalledTimes(1);
	});

	it("attaches a connected Coder MCP and uses the MCP prompt", async () => {
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			MockMCPServerConfig,
			coderMcp,
		]);
		vi.spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});

		const { result, create } = open([chat("p")], undefined);
		await result;

		expect(API.experimental.getMCPServerConfigs).toHaveBeenCalledWith(
			MockChat.organization_id,
		);
		const request = create.mock.calls[0]?.[0];
		expect(request?.mcp_server_ids).toEqual(["mcp-coder"]);
		expect(request?.system_prompt).toContain("coder_get_chat");
	});

	it("still creates the chat without a workspace when the lookup fails but the MCP is attached", async () => {
		const { toast } = await import("sonner");
		vi.mocked(toast.error).mockClear();
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			coderMcp,
		]);
		vi.spyOn(API, "getWorkspaces").mockRejectedValue(new Error("offline"));

		const { result, create } = open([chat("p")], undefined);

		expect(await result).toBe("created");
		expect(toast.error).not.toHaveBeenCalled();
		const request = create.mock.calls[0]?.[0];
		expect(request?.workspace_id).toBeUndefined();
		expect(request?.mcp_server_ids).toEqual(["mcp-coder"]);
	});

	it("leaves the MCP off when it is disconnected, absent or unknown", async () => {
		vi.spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});
		const createWithServers = async (
			servers: () => Promise<(typeof coderMcp)[]>,
		) => {
			vi.spyOn(API.experimental, "getMCPServerConfigs").mockImplementation(
				servers,
			);
			const { result, create } = open([chat("p")], undefined);
			await result;
			return create.mock.calls[0]?.[0];
		};

		const disconnected = await createWithServers(() =>
			Promise.resolve([{ ...coderMcp, auth_connected: false }]),
		);
		expect(disconnected?.mcp_server_ids).toBeUndefined();
		expect(disconnected?.system_prompt).not.toContain("coder_");

		const absent = await createWithServers(() =>
			Promise.resolve([MockMCPServerConfig]),
		);
		expect(absent?.mcp_server_ids).toBeUndefined();

		const failed = await createWithServers(() =>
			Promise.reject(new Error("503")),
		);
		expect(failed?.mcp_server_ids).toBeUndefined();
		expect(failed?.system_prompt).not.toContain("coder_");
	});

	it("shares one creation between concurrent opens of the same key on one client", async () => {
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		const lookup = createDeferred<{ workspaces: never[]; count: number }>();
		vi.spyOn(API, "getWorkspaces").mockReturnValue(lookup.promise);
		const queryClient = new QueryClient();
		const card = cardFor([chat("p")]);
		const create = vi.fn().mockResolvedValue({ ...MockChat, id: "created" });
		const rename = vi.fn().mockResolvedValue(undefined);
		const args = {
			spec: (tools: AssistantTools) => cardAssistantSpec(card, tools),
			existingId: undefined,
			create,
			rename,
			queryClient,
		};

		const first = openAssistant(args);
		const second = openAssistant(args);
		const elsewhere = openAssistant({
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
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
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
			spec: (tools: AssistantTools) => cardAssistantSpec(card, tools),
			create,
			rename: vi.fn().mockResolvedValue(undefined),
			queryClient,
		};

		await openAssistant({ ...args, existingId: undefined });
		// What the page computes from the list on its next render.
		const listed =
			queryClient.getQueryData<{ pages: Chat[][] }>(boardChatsKey)?.pages[0] ??
			[];
		const existingId = assistantIds(listed).get(card.id);
		const again = await openAssistant({ ...args, existingId });

		expect(existingId).toBe("created");
		expect(again).toBe("created");
		expect(create).toHaveBeenCalledTimes(1);
	});
});
