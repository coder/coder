import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import type { BoardCard } from "./boardLabels";
import { NotesSection } from "./NotesSection";

const notes = [
	{ index: 0, timestamp: 1000, text: "first note" },
	{ index: 3, timestamp: 2000, text: "later note" },
];

const card: BoardCard = {
	id: MockChat.id,
	title: "Epic",
	column: "Inbox",
	color: undefined,
	primary: MockChat,
	members: [MockChat],
	comments: notes,
};

const renderNotes = () => {
	const handlers = { onAdd: vi.fn(), onEdit: vi.fn(), onRemove: vi.fn() };
	renderComponent(
		<NotesSection card={card} noteDrop={undefined} {...handlers} />,
	);
	return handlers;
};

describe("NotesSection", () => {
	it("adds a note on Enter and clears the composer on Escape", async () => {
		const user = userEvent.setup();
		const { onAdd } = renderNotes();
		const composer = screen.getByRole("textbox", {
			name: "Add a note to Epic",
		});

		await user.type(composer, "discarded");
		await user.keyboard("{Escape}");
		expect(composer).toHaveValue("");
		expect(onAdd).not.toHaveBeenCalled();

		await user.type(composer, "  new note {Enter}");
		expect(onAdd).toHaveBeenCalledWith("new note");
	});

	it("adds a note from the arrow button and ignores blank text", async () => {
		const user = userEvent.setup();
		const { onAdd } = renderNotes();
		const composer = screen.getByRole("textbox", {
			name: "Add a note to Epic",
		});

		await user.click(screen.getByRole("button", { name: "Save note" }));
		expect(onAdd).not.toHaveBeenCalled();

		await user.type(composer, "from button");
		await user.click(screen.getByRole("button", { name: "Save note" }));
		expect(onAdd).toHaveBeenCalledWith("from button");
	});

	it("edits an existing note by its index", async () => {
		const user = userEvent.setup();
		const { onEdit } = renderNotes();

		await user.click(screen.getAllByRole("button", { name: "Edit note" })[1]);
		const field = screen.getByRole("textbox", { name: "Note text" });
		expect(field).toHaveValue("later note");
		await user.clear(field);
		await user.type(field, "changed{Enter}");

		expect(onEdit).toHaveBeenCalledWith(3, "changed");
	});

	it("removes a note after confirming", async () => {
		const user = userEvent.setup();
		const { onRemove } = renderNotes();

		await user.click(screen.getAllByRole("button", { name: "Delete note" })[0]);
		await user.click(await screen.findByRole("button", { name: "Delete" }));

		expect(onRemove).toHaveBeenCalledWith(0);
	});
});
