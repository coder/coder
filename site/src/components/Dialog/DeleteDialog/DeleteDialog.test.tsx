import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import { DeleteDialog } from "./DeleteDialog";

describe("DeleteDialog", () => {
	it("keeps focus in the dialog when submitting an incorrect confirmation with Enter", async () => {
		const user = userEvent.setup();
		const onConfirm = vi.fn();
		const onCancel = vi.fn();
		const onAncestorKeyDown = vi.fn();

		render(
			<div onKeyDown={onAncestorKeyDown}>
				<DeleteDialog
					isOpen
					onConfirm={onConfirm}
					onCancel={onCancel}
					entity="workspace"
					name="my-workspace"
				/>
			</div>,
		);

		const input = screen.getByLabelText("Name of the workspace to delete");
		await user.type(input, "wrong-name");
		await user.keyboard("{Enter}");

		expect(onConfirm).not.toHaveBeenCalled();
		expect(onCancel).not.toHaveBeenCalled();
		expect(onAncestorKeyDown).not.toHaveBeenCalledWith(
			expect.objectContaining({ key: "Enter" }),
		);
		expect(input).toHaveFocus();
		expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled();
	});
});
