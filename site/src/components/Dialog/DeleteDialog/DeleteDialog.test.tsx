import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import { DeleteDialog } from "./DeleteDialog";

const renderDialog = () => {
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

	return {
		onConfirm,
		onCancel,
		input: screen.getByLabelText("Name of the workspace to delete"),
	};
};

describe("DeleteDialog", () => {
	it("marks the input invalid instead of confirming when Enter is pressed with a wrong name", async () => {
		const user = userEvent.setup();
		const { onConfirm, onCancel, input } = renderDialog();

		await user.type(input, "wrong-name{Enter}");

		expect(onConfirm).not.toHaveBeenCalled();
		expect(onCancel).not.toHaveBeenCalled();
		expect(input).toHaveFocus();
		expect(input).toHaveAttribute("aria-invalid", "true");
		expect(screen.getByRole("alert")).toHaveTextContent("does not match");
	});

	it("clears the submitted error when the user edits the name", async () => {
		const user = userEvent.setup();
		const { input } = renderDialog();

		await user.type(input, "wrong-name{Enter}x");

		expect(input).toHaveAttribute("aria-invalid", "false");
	});

	it("confirms on Enter when the name matches", async () => {
		const user = userEvent.setup();
		const { onConfirm, input } = renderDialog();

		await user.type(input, "my-workspace{Enter}");

		expect(onConfirm).toHaveBeenCalledTimes(1);
	});
});
