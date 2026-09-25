import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type FC, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { InlineEdit } from "./InlineEdit";

const renderEdit = () => {
	const onSave = vi.fn();
	const onDone = vi.fn();
	renderComponent(
		<>
			<InlineEdit
				value="Before"
				ariaLabel="title"
				onSave={onSave}
				onDone={onDone}
			/>
			<button type="button">elsewhere</button>
		</>,
	);
	return {
		onSave,
		onDone,
		field: screen.getByRole("textbox", { name: "title" }),
	};
};

describe("InlineEdit", () => {
	it("saves a changed value on Enter", async () => {
		const user = userEvent.setup();
		const { onSave, onDone, field } = renderEdit();
		await user.clear(field);
		await user.type(field, "After{Enter}");
		expect(onSave).toHaveBeenCalledWith("After");
		expect(onDone).toHaveBeenCalledTimes(1);
		expect(field).toHaveValue("Before");
	});

	it("saves a changed value when focus leaves", async () => {
		const user = userEvent.setup();
		const { onSave, onDone, field } = renderEdit();
		await user.clear(field);
		await user.type(field, "After");
		await user.click(screen.getByRole("button", { name: "elsewhere" }));
		expect(onSave).toHaveBeenCalledWith("After");
		expect(onDone).toHaveBeenCalledTimes(1);
	});

	it("discards the edit on Escape", async () => {
		const user = userEvent.setup();
		const { onSave, onDone, field } = renderEdit();
		await user.clear(field);
		await user.type(field, "After{Escape}");
		expect(onSave).not.toHaveBeenCalled();
		expect(onDone).toHaveBeenCalledTimes(1);
	});

	it("saves nothing when Escape unmounts the focused field", async () => {
		const user = userEvent.setup();
		const onSave = vi.fn();
		const Parent: FC = () => {
			const [editing, setEditing] = useState(true);
			if (!editing) return <span>Before</span>;
			return (
				<InlineEdit
					value="Before"
					ariaLabel="title"
					onSave={onSave}
					onDone={() => setEditing(false)}
				/>
			);
		};
		renderComponent(<Parent />);
		const field = screen.getByRole("textbox", { name: "title" });
		await user.clear(field);
		await user.type(field, "After{Escape}");
		await user.click(document.body);
		expect(onSave).not.toHaveBeenCalled();
	});

	it("does not save an unchanged or blank value", async () => {
		const user = userEvent.setup();
		const { onSave, onDone, field } = renderEdit();
		await user.type(field, "{Enter}");
		await user.clear(field);
		await user.type(field, "   {Enter}");
		expect(onSave).not.toHaveBeenCalled();
		expect(onDone).toHaveBeenCalledTimes(2);
	});
});
