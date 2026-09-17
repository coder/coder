import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { boardAssistantSpec, cardAssistantSpec } from "./assistantSpecs";
import type { BoardState } from "./boardApi";
import { addCommentLabels, buildCards, buildColumns } from "./boardLabels";

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

const stateOf = (chats: readonly Chat[]): BoardState => {
	const cards = buildCards(chats);
	return {
		cards,
		columns: buildColumns(cards, [], []),
		storage: {
			columnOrder: [],
			emptyColumns: [],
			windows: [],
			effortFilter: null,
		},
	};
};

describe("cardAssistantSpec", () => {
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
		const spec = cardAssistantSpec(card, { coderMcp: false });
		expect(spec).toMatchObject({
			key: "p",
			title: "Assistant: Epic",
			organizationId: MockChat.organization_id,
		});
		expect(spec.systemPrompt).toContain("assistant for one card");
		expect(spec.systemPrompt).toContain(
			"Card id (primary chat): p. Members: p, m.",
		);
		expect(spec.systemPrompt).toContain("notes");
		expect(spec.systemPrompt).toContain(
			"Propose, then act only on an explicit yes.",
		);
		expect(spec.systemPrompt).not.toContain("board/effort");
		expect(spec.systemPrompt).not.toContain("Merge two cards");
		const text = spec.snapshot;
		expect(text).toContain('Assistant for card "Epic"');
		expect(text).toContain("Column: Doing");
		expect(text.indexOf("sooner")).toBeLessThan(text.indexOf("later"));
		expect(text).toContain("1. Chat p\n   id: p\n   status: waiting");
		expect(text).toContain("2. Chat m\n   id: m\n   status: running");
		expect(text).toContain("summary: done");
		expect(text).toContain("Notes:\n");
		expect(
			cardAssistantSpec(cardFor([chat("s")]), { coderMcp: false }).snapshot,
		).toContain("Notes: none");
	});
});

describe("boardAssistantSpec", () => {
	it("keys the one board assistant for the given organization", () => {
		const chats = [chat("a"), chat("b")];
		const spec = boardAssistantSpec(stateOf(chats), "org-1", {
			coderMcp: false,
		});
		expect(spec).toMatchObject({
			key: "board",
			title: "Board assistant",
			organizationId: "org-1",
		});
		expect(spec.systemPrompt).toContain("PATCH");
		expect(spec.systemPrompt).toContain("board/comment.N.M");
		expect(spec.systemPrompt).toContain(
			"set board/comment.N.timestamp to the current Unix ms",
		);
		expect(spec.systemPrompt).toContain('a note "Merged card: <title>"');
		expect(spec.systemPrompt).toContain(
			"oldest remaining member by created_at becomes primary",
		);
		expect(spec.systemPrompt).toContain(
			"Propose, then act only on an explicit yes.",
		);
	});

	it("names the MCP tools and the API gaps only when the Coder MCP is attached", () => {
		const state = stateOf([chat("a")]);
		const withMcp = boardAssistantSpec(state, "org-1", {
			coderMcp: true,
		}).systemPrompt;
		expect(withMcp).toContain("coder_get_chat");
		expect(withMcp).toContain(
			"only for these gaps: label writes (PATCH), chat title writes, created_at, summary, diff, cost",
		);
		expect(withMcp).toContain('find_tools with queries ["chat"]');
		expect(withMcp).toContain("the first time you need one of the API gaps");

		const curlOnly = boardAssistantSpec(state, "org-1", {
			coderMcp: false,
		}).systemPrompt;
		expect(curlOnly).not.toContain("coder_");
		expect(curlOnly).toContain(
			"the first time you need to read or write anything",
		);
		expect(
			cardAssistantSpec(cardFor([chat("a")]), { coderMcp: true }).systemPrompt,
		).toContain("coder_get_chat_messages");
	});

	it("snapshots every card with its primary id, column, chats and notes", () => {
		const chats = [
			chat("p", {
				"board/title": "Epic",
				"board/column": "Doing",
				"board/color": "sky",
				"board/effort.0": "Q3",
				"board/effort.1": "This week",
				...addCommentLabels(addCommentLabels({}, "one", 1), "two", 2),
			}),
			{ ...chat("m", { "board/group": "p" }), status: "running" as const },
			chat("s"),
		];
		const spec = boardAssistantSpec(stateOf(chats), "org-1", {
			coderMcp: false,
		});
		const text = spec.snapshot;
		expect(text).toContain("Columns: Inbox, Doing");
		expect(text).toContain(
			"Card p | column: Doing | title: Epic | color: sky | efforts: Q3, This week\n  chats: Chat p (p) status: waiting; Chat m (m) status: running\n  notes: one | two",
		);
		expect(text).toContain(
			"Card s | column: Inbox | title: Chat s | color: none | efforts: none\n  chats: Chat s (s) status: waiting\n  notes: none",
		);
	});
});
