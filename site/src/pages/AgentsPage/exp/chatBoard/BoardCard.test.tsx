import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
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
			isDropTarget={false}
			noteDrop={undefined}
			{...handlers}
		/>,
	);
	return { card, ...handlers };
};

describe("BoardCard", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});
	it("reports an edited title as the card title", async () => {
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

	it("opens the chat from its icon with the card as anchor", async () => {
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
		const rect = (top: number) => new DOMRect(0, top, 0, 0);
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
