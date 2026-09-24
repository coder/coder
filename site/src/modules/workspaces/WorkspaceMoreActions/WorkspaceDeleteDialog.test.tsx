import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockWorkspace } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";

const renderDialog = () => {
	const onConfirm = vi.fn();
	const onCancel = vi.fn();

	render(
		<WorkspaceDeleteDialog
			workspace={MockWorkspace}
			canDeleteFailedWorkspace={false}
			isOpen
			onConfirm={onConfirm}
			onCancel={onCancel}
		/>,
	);

	return { onConfirm, onCancel };
};

describe("WorkspaceDeleteDialog", () => {
	it("marks the input invalid instead of confirming when Enter is pressed with a wrong name", async () => {
		const user = userEvent.setup();
		const { onConfirm, onCancel } = renderDialog();

		const input = screen.getByLabelText("Workspace name");
		await user.type(input, "wrong name{Enter}");

		expect(onConfirm).not.toHaveBeenCalled();
		expect(onCancel).not.toHaveBeenCalled();
		expect(input).toHaveFocus();
		expect(input).toHaveAttribute("aria-invalid", "true");
	});

	it("confirms on Enter when the name matches", async () => {
		const user = userEvent.setup();
		const { onConfirm } = renderDialog();

		await user.type(
			screen.getByLabelText("Workspace name"),
			`${MockWorkspace.name}{Enter}`,
		);

		expect(onConfirm).toHaveBeenCalledWith(false);
	});
});
