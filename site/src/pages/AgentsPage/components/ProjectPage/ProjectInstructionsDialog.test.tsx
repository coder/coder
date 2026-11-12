import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MockChatProjectInstructions } from "#/testHelpers/entities";
import { ProjectInstructionsDialog } from "./ProjectInstructionsDialog";

type DialogProps = React.ComponentProps<typeof ProjectInstructionsDialog>;

const renderDialog = (props: Partial<DialogProps> = {}) => {
	const allProps: DialogProps = {
		open: true,
		onOpenChange: vi.fn(),
		instructions: "",
		isSaving: false,
		isDeleting: false,
		saveError: undefined,
		deleteError: undefined,
		onResetErrors: vi.fn(),
		onSave: vi.fn(),
		onDelete: vi.fn(),
		...props,
	};
	const { rerender } = render(<ProjectInstructionsDialog {...allProps} />);
	return {
		user: userEvent.setup(),
		props: allProps,
		rerenderWithInstructions: (instructions: string) =>
			rerender(
				<ProjectInstructionsDialog {...allProps} instructions={instructions} />,
			),
	};
};

describe("ProjectInstructionsDialog", () => {
	it("does not save text that is blank once the server normalizes it", async () => {
		const { user, props } = renderDialog();

		fireEvent.change(screen.getByRole("textbox", { name: "Instructions" }), {
			target: { value: "\u200B\u2060 \n\n\n\u200B" },
		});
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSave).not.toHaveBeenCalled();
	});

	it("does not save edits the server would discard", async () => {
		const { user, props } = renderDialog({ instructions: "One.\n\nTwo." });
		const textbox = screen.getByRole("textbox", { name: "Instructions" });

		// Trailing whitespace on a line and a third newline are both
		// removed when the server stores the text.
		fireEvent.change(textbox, { target: { value: "One.  \n\n\nTwo." } });
		await user.click(screen.getByRole("button", { name: "Save" }));
		expect(props.onSave).not.toHaveBeenCalled();

		await user.type(textbox, " Three.");
		await user.click(screen.getByRole("button", { name: "Save" }));
		expect(props.onSave).toHaveBeenCalledWith("One.  \n\n\nTwo. Three.");
	});

	it("blocks Save when the saved instructions change while editing, until the user loads them", async () => {
		const { user, props, rerenderWithInstructions } = renderDialog({
			instructions: MockChatProjectInstructions.instructions,
		});
		const textbox = screen.getByRole("textbox", { name: "Instructions" });
		await user.type(textbox, " My edit.");

		rerenderWithInstructions("Edited in another tab.");
		await user.click(screen.getByRole("button", { name: "Clear" }));
		await user.click(screen.getByRole("button", { name: "Save" }));
		expect(props.onSave).not.toHaveBeenCalled();
		expect(props.onDelete).not.toHaveBeenCalled();

		await user.click(screen.getByRole("button", { name: "Load latest" }));
		expect(textbox).toHaveValue("Edited in another tab.");
		await user.click(screen.getByRole("button", { name: "Clear" }));
		await user.click(screen.getByRole("button", { name: "Save" }));
		expect(props.onDelete).toHaveBeenCalledTimes(1);
	});

	it("removes the instructions only once the cleared draft is saved", async () => {
		const { user, props } = renderDialog({
			instructions: MockChatProjectInstructions.instructions,
		});
		const textbox = screen.getByRole("textbox", { name: "Instructions" });

		await user.click(screen.getByRole("button", { name: "Clear" }));
		expect(textbox).toHaveValue("");
		expect(textbox).toHaveFocus();
		expect(props.onDelete).not.toHaveBeenCalled();

		await user.click(screen.getByRole("button", { name: "Save" }));
		expect(props.onDelete).toHaveBeenCalledTimes(1);
		expect(props.onSave).not.toHaveBeenCalled();
	});

	it("clears a failed save's error before removing the instructions", async () => {
		const { user, props } = renderDialog({
			instructions: MockChatProjectInstructions.instructions,
			saveError: new Error("Save failed."),
		});

		await user.click(screen.getByRole("button", { name: "Clear" }));
		vi.mocked(props.onResetErrors).mockClear();
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onResetErrors).toHaveBeenCalledTimes(1);
		expect(
			vi.mocked(props.onResetErrors).mock.invocationCallOrder[0],
		).toBeLessThan(vi.mocked(props.onDelete).mock.invocationCallOrder[0]);
	});

	it("clears a failed delete's error before saving", async () => {
		const { user, props } = renderDialog({
			instructions: MockChatProjectInstructions.instructions,
			deleteError: new Error("Delete failed."),
		});
		await user.type(
			screen.getByRole("textbox", { name: "Instructions" }),
			" More.",
		);
		vi.mocked(props.onResetErrors).mockClear();

		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onResetErrors).toHaveBeenCalledTimes(1);
		expect(
			vi.mocked(props.onResetErrors).mock.invocationCallOrder[0],
		).toBeLessThan(vi.mocked(props.onSave).mock.invocationCallOrder[0]);
	});
});
