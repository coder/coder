import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { BoardCard } from "./BoardCard";
import { buildCards } from "./boardLabels";

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	title: `Chat ${id}`,
	labels,
});

const renderCard = (chats: readonly Chat[]) => {
	const [card] = buildCards(chats);
	if (!card) throw new Error("card missing");
	const handlers = {
		onSetTitle: vi.fn(),
		onSetColor: vi.fn(),
		onRenameChat: vi.fn(),
		onAssistant: vi.fn(),
		onNewChat: vi.fn(),
		onSetEfforts: vi.fn(),
		onFilterEffort: vi.fn(),
		onRemoveFromGroup: vi.fn(),
		onOpen: vi.fn(),
		onPreview: vi.fn(),
		onPreviewEnd: vi.fn(),
		onAddNote: vi.fn(),
		onEditNote: vi.fn(),
		onRemoveNote: vi.fn(),
	} satisfies Partial<ComponentProps<typeof BoardCard>>;
	// No DndContext: its default sensors have no activation distance, so a
	// pointerdown on the header would start a drag and swallow the click.
	// Outside a context the dnd-kit hooks are inert, like a click that stays
	// under the real board's distance threshold.
	renderComponent(
		<BoardCard
			card={card}
			openChatIds={new Set()}
			isMergeTarget={false}
			noteDrop={undefined}
			knownEfforts={["Q3", "This week"]}
			{...handlers}
		/>,
	);
	return { card, ...handlers };
};

describe("BoardCard", () => {
	it("reports an edited title as the card title, single chat or group", async () => {
		const user = userEvent.setup();
		const { onRenameChat, onSetTitle } = renderCard([chat("p")]);

		await user.click(screen.getByRole("button", { name: "Chat p" }));
		const field = screen.getByRole("textbox", { name: "title" });
		await user.clear(field);
		await user.type(field, "Renamed{Enter}");

		// Whether this renames the chat or sets a label is the board's call.
		expect(onSetTitle).toHaveBeenCalledWith("Renamed");
		expect(onRenameChat).not.toHaveBeenCalled();
	});

	it("picks a color from the stripe swatches", async () => {
		const user = userEvent.setup();
		const { onSetColor } = renderCard([chat("p", { "board/color": "red" })]);

		await user.click(screen.getByRole("button", { name: "Color of Chat p" }));
		await user.click(screen.getByRole("button", { name: "sky" }));
		expect(onSetColor).toHaveBeenCalledWith("sky");

		await user.click(screen.getByRole("button", { name: "Color of Chat p" }));
		await user.click(screen.getByRole("button", { name: "No color" }));
		expect(onSetColor).toHaveBeenCalledWith(undefined);
	});

	it("opens the assistant from the actions menu", async () => {
		const user = userEvent.setup();
		const { onAssistant } = renderCard([chat("p")]);

		await user.click(
			screen.getByRole("button", { name: "Actions for Chat p" }),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "Assistant" }),
		);

		expect(onAssistant).toHaveBeenCalledTimes(1);
	});

	it("starts a new chat in the card from the actions menu", async () => {
		const user = userEvent.setup();
		const { onNewChat } = renderCard([chat("p")]);

		await user.click(
			screen.getByRole("button", { name: "Actions for Chat p" }),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "New chat in card" }),
		);

		expect(onNewChat).toHaveBeenCalledTimes(1);
	});

	it("edits its efforts from the menu, toggling known ones and coining new ones", async () => {
		const user = userEvent.setup();
		const { onSetEfforts } = renderCard([
			chat("p", { "board/effort.0": "Q3" }),
		]);

		await user.click(
			screen.getByRole("button", { name: "Actions for Chat p" }),
		);
		// Opened from the keyboard: without layout, jsdom cannot tell a pointer
		// heading into the sub menu from one leaving it, and would close it.
		(await screen.findByRole("menuitem", { name: "Efforts" })).focus();
		await user.keyboard("{ArrowRight}");
		await user.click(
			await screen.findByRole("menuitemcheckbox", { name: "This week" }),
		);
		expect(onSetEfforts).toHaveBeenCalledWith(["Q3", "This week"]);

		// The sub menu stayed open: the next toggle needs no reopening.
		await user.click(screen.getByRole("menuitemcheckbox", { name: "Q3" }));
		expect(onSetEfforts).toHaveBeenLastCalledWith([]);

		await user.type(
			screen.getByRole("textbox", { name: "New effort" }),
			"Launch{Enter}",
		);
		expect(onSetEfforts).toHaveBeenLastCalledWith(["Q3", "Launch"]);
	});

	it("filters the board by an effort from its tag", async () => {
		const user = userEvent.setup();
		const { onFilterEffort } = renderCard([
			chat("p", { "board/effort.0": "Q3" }),
		]);

		await user.click(screen.getByRole("button", { name: "Filter by Q3" }));

		expect(onFilterEffort).toHaveBeenCalledWith("Q3");
	});

	it("opens the chat from its icon with the row as anchor", async () => {
		const user = userEvent.setup();
		const { card, onOpen } = renderCard([chat("p")]);

		// The header surface shares the icon's label; the icon alone has the title.
		await user.click(screen.getByTitle("Open chat"));

		expect(onOpen).toHaveBeenCalledWith(
			card.primary,
			expect.objectContaining({ top: expect.any(Number) }),
		);
	});

	it("anchors a group row's window to that row, not the card", async () => {
		const user = userEvent.setup();
		const { card, onOpen, onPreview } = renderCard([
			chat("p", { "board/group": "p" }),
			chat("m", { "board/group": "p" }),
		]);
		const rect = (top: number) => ({ top }) as DOMRect;
		vi.spyOn(
			screen.getByRole("article"),
			"getBoundingClientRect",
		).mockReturnValue(rect(10));
		const [, secondRow] = screen.getAllByRole("listitem");
		vi.spyOn(secondRow, "getBoundingClientRect").mockReturnValue(rect(120));

		const opener = screen.getAllByTitle("Open chat")[1];
		await user.hover(opener);
		await user.click(opener);

		expect(onPreview).toHaveBeenCalledWith(card.members[1], rect(120));
		expect(onOpen).toHaveBeenCalledWith(card.members[1], rect(120));
	});
});
