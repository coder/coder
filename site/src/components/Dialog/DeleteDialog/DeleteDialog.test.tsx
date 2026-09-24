import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import { DeleteDialog } from "./DeleteDialog";

describe("DeleteDialog", () => {
	it("keeps focus in the dialog when submitting an incorrect confirmation with Enter", async () => {
		const user = userEvent.setup();
		const onConfirm = vi.fn();
		const onCancel = vi.fn();

		render(
			<DeleteDialog
				isOpen
				onConfirm={onConfirm}
				onCancel={onCancel}
				entity="workspace"
				name="my-workspace"
			/>,
		);

		const input = screen.getByLabelText("Name of the workspace to delete");
		await user.type(input, "wrong-name");
		await user.keyboard("{Enter}");

		expect(onConfirm).not.toHaveBeenCalled();
		expect(onCancel).not.toHaveBeenCalled();
		expect(input).toHaveFocus();
		expect(input).toHaveAttribute("aria-invalid", "true");
		expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled();
	});
});
