import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { BoardColumn, NewColumn } from "./BoardColumn";
import { buildCards, buildColumns, INBOX_COLUMN } from "./boardLabels";

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	title: `Chat ${id}`,
	labels,
});

const renderColumn = (name: string) => {
	const cards = buildCards([chat("p", { "board/column": name })]);
	const [column] = buildColumns(cards, [INBOX_COLUMN, name], []).filter(
		(c) => c.name === name,
	);
	if (!column) throw new Error("column missing");
	const onRename = vi.fn();
	const onDelete = vi.fn();
	const onNewChat = vi.fn();
	const noop = vi.fn();
	// No DndContext, see BoardCard.test.tsx.
	renderComponent(
		<BoardColumn
			column={column}
			openChatIds={new Set()}
			dropTarget={null}
			onRename={onRename}
			onDelete={onDelete}
			onNewChat={onNewChat}
			onSetCardTitle={noop}
			onSetCardColor={noop}
			onRenameChat={noop}
			onAssistant={noop}
			onNewChatInCard={noop}
			onRemoveFromGroup={noop}
			onOpen={noop}
			onPreview={noop}
			onPreviewEnd={noop}
			onAddNote={noop}
			onEditNote={noop}
			onRemoveNote={noop}
		/>,
	);
	return { onRename, onDelete, onNewChat };
};

describe("BoardColumn", () => {
	it("starts a new chat in the column from the header plus", async () => {
		const user = userEvent.setup();
		const { onNewChat } = renderColumn("Doing");

		await user.click(screen.getByRole("button", { name: "New chat in Doing" }));

		expect(onNewChat).toHaveBeenCalledTimes(1);
	});

	it("renames from the title and deletes from the menu", async () => {
		const user = userEvent.setup();
		const { onRename, onDelete } = renderColumn("Doing");

		await user.click(screen.getByRole("button", { name: "Doing" }));
		const field = screen.getByRole("textbox", { name: "Doing column name" });
		await user.clear(field);
		await user.type(field, "Done{Enter}");
		expect(onRename).toHaveBeenCalledWith("Done");

		await user.click(
			screen.getByRole("button", { name: "Actions for Doing column" }),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "Delete column" }),
		);
		expect(onDelete).toHaveBeenCalledTimes(1);
	});
});

describe("NewColumn", () => {
	it("creates on Enter and cancels on Escape", async () => {
		const user = userEvent.setup();
		const onCreate = vi.fn();
		const onCancel = vi.fn();
		renderComponent(<NewColumn onCreate={onCreate} onCancel={onCancel} />);
		const field = screen.getByRole("textbox", { name: "New column name" });

		await user.type(field, "Review{Enter}");
		expect(onCreate).toHaveBeenCalledWith("Review");
		expect(onCancel).toHaveBeenCalledTimes(1);

		await user.type(field, "{Escape}");
		expect(onCreate).toHaveBeenCalledTimes(1);
		expect(onCancel).toHaveBeenCalledTimes(2);
	});
});
