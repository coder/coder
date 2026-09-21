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

const renderNotes = (comments = notes) => {
	const handlers = { onAdd: vi.fn(), onEdit: vi.fn(), onRemove: vi.fn() };
	const view = renderComponent(
		<NotesSection
			card={{ ...card, comments }}
			noteDrop={undefined}
			{...handlers}
		/>,
	);
	const rerender = (next: BoardCard["comments"]) =>
		view.rerender(
			<NotesSection
				card={{ ...card, comments: next }}
				noteDrop={undefined}
				{...handlers}
			/>,
		);
	return { ...handlers, rerender };
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
		expect(onEdit).toHaveBeenCalledTimes(1);
	});

	it("keeps an open editor on its note when the notes are renumbered", async () => {
		const user = userEvent.setup();
		const { onEdit, rerender } = renderNotes([
			{ index: 0, timestamp: 1000, text: "first note" },
			{ index: 1, timestamp: 2000, text: "later note" },
		]);

		await user.click(screen.getAllByRole("button", { name: "Edit note" })[1]);
		// A sibling was dragged above it: same notes, indices swapped.
		rerender([
			{ index: 0, timestamp: 2000, text: "later note" },
			{ index: 1, timestamp: 1000, text: "first note" },
		]);
		const field = screen.getByRole("textbox", { name: "Note text" });
		expect(field).toHaveValue("later note");
		await user.clear(field);
		await user.type(field, "changed{Enter}");

		expect(onEdit).toHaveBeenCalledWith(0, "changed");
	});

	it("saves an edited note once from the arrow button", async () => {
		const user = userEvent.setup();
		const { onEdit } = renderNotes();

		await user.click(screen.getAllByRole("button", { name: "Edit note" })[0]);
		const field = screen.getByRole("textbox", { name: "Note text" });
		await user.clear(field);
		await user.type(field, "changed");
		await user.click(screen.getAllByRole("button", { name: "Save note" })[0]);

		expect(onEdit).toHaveBeenCalledTimes(1);
		expect(onEdit).toHaveBeenCalledWith(0, "changed");
	});

	it("discards an edit on Escape and reopens with the stored text", async () => {
		const user = userEvent.setup();
		const { onEdit } = renderNotes();

		await user.click(screen.getAllByRole("button", { name: "Edit note" })[0]);
		const field = screen.getByRole("textbox", { name: "Note text" });
		await user.type(field, " discarded{Escape}");
		await user.click(document.body);
		expect(onEdit).not.toHaveBeenCalled();

		await user.click(screen.getAllByRole("button", { name: "Edit note" })[0]);
		expect(screen.getByRole("textbox", { name: "Note text" })).toHaveValue(
			"first note",
		);
	});

	it("clears the composer after a note is added", async () => {
		const user = userEvent.setup();
		const { onAdd } = renderNotes();
		const composer = screen.getByRole("textbox", {
			name: "Add a note to Epic",
		});

		await user.type(composer, "one{Enter}");
		expect(composer).toHaveValue("");
		await user.type(composer, "two{Enter}");
		expect(onAdd.mock.calls).toEqual([["one"], ["two"]]);
	});

	it("removes a note after confirming", async () => {
		const user = userEvent.setup();
		const { onRemove } = renderNotes();

		await user.click(screen.getAllByRole("button", { name: "Delete note" })[0]);
		await user.click(await screen.findByRole("button", { name: "Delete" }));

		expect(onRemove).toHaveBeenCalledWith(0);
	});
});
