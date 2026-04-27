import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { DeleteDialog } from "./DeleteDialog";

const inputTestId = "delete-dialog-name-confirmation";

async function fillInputField(inputElement: HTMLElement, text: string) {
	// 2023-10-06: MUI's TextField can update state after a typing event fires,
	// and React Testing Library is not able to catch it. Manually wrapping the
	// event keeps React DOM aware of the changes.
	return act(() => userEvent.type(inputElement, text));
}

describe("DeleteDialog", () => {
	it("disables confirm button when the text field is empty", () => {
		renderComponent(
			<DeleteDialog
				isOpen
				onConfirm={vi.fn()}
				onCancel={vi.fn()}
				entity="template"
				name="MyTemplate"
			/>,
		);

		const confirmButton = screen.getByRole("button", { name: "Delete" });
		expect(confirmButton).toBeDisabled();
	});

	it("disables confirm button when the text field is filled incorrectly", async () => {
		renderComponent(
			<DeleteDialog
				isOpen
				onConfirm={vi.fn()}
				onCancel={vi.fn()}
				entity="template"
				name="MyTemplate"
			/>,
		);

		const textField = screen.getByTestId(inputTestId);
		await fillInputField(textField, "MyTemplateButWrong");

		const confirmButton = screen.getByRole("button", { name: "Delete" });
		expect(confirmButton).toBeDisabled();
	});

	it("enables confirm button when the text field is filled correctly", async () => {
		renderComponent(
			<DeleteDialog
				isOpen
				onConfirm={vi.fn()}
				onCancel={vi.fn()}
				entity="template"
				name="MyTemplate"
			/>,
		);

		const textField = screen.getByTestId(inputTestId);
		await fillInputField(textField, "MyTemplate");

		const confirmButton = screen.getByRole("button", { name: "Delete" });
		expect(confirmButton).not.toBeDisabled();
	});
});
