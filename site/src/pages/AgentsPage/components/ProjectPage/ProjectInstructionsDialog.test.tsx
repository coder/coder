import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
	MockChatProjectInstructions,
	mockApiError,
} from "#/testHelpers/entities";
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
		onDraftChange: vi.fn(),
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
	it("does not save text that is blank once invisible characters are stripped", () => {
		renderDialog();

		fireEvent.change(screen.getByRole("textbox", { name: "Instructions" }), {
			target: { value: "\u200B\u2060 \u200B" },
		});

		expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
	});

	it.each([
		{ errorProp: "saveError", message: "Failed to save instructions." },
		{ errorProp: "deleteError", message: "Failed to delete instructions." },
	] as const)(
		"labels a $errorProp without an API message by its action",
		({ errorProp, message }) => {
			// For example, a proxy error page instead of a coderd response.
			renderDialog({
				instructions: MockChatProjectInstructions.instructions,
				[errorProp]: mockApiError({ message: "" }),
			});

			expect(screen.getByRole("dialog")).toHaveTextContent(message);
		},
	);

	it("blocks Save and Delete when the saved instructions change while editing, until the user loads them", async () => {
		const { user, rerenderWithInstructions } = renderDialog({
			instructions: MockChatProjectInstructions.instructions,
		});
		const textbox = screen.getByRole("textbox", { name: "Instructions" });
		await user.type(textbox, " My edit.");

		rerenderWithInstructions("Edited in another tab.");

		expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
		expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled();
		await user.click(screen.getByRole("button", { name: "Load latest" }));
		expect(textbox).toHaveValue("Edited in another tab.");
		expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
		expect(screen.getByRole("button", { name: "Delete" })).toBeEnabled();
	});
});
