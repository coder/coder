import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockWorkspace } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";

const renderDialog = () => {
	const onConfirm = vi.fn();
	const onCancel = vi.fn();
	const onAncestorKeyDown = vi.fn();

	render(
		// Stands in for a clickable table row that navigates on Enter.
		<div onKeyDown={onAncestorKeyDown}>
			<WorkspaceDeleteDialog
				workspace={MockWorkspace}
				canDeleteFailedWorkspace={false}
				isOpen
				onConfirm={onConfirm}
				onCancel={onCancel}
			/>
		</div>,
	);

	return { onConfirm, onCancel, onAncestorKeyDown };
};

describe("WorkspaceDeleteDialog", () => {
	it("does not confirm or leak Enter to ancestors when the name is wrong", async () => {
		const user = userEvent.setup();
		const { onConfirm, onCancel, onAncestorKeyDown } = renderDialog();

		const input = screen.getByLabelText("Workspace name");
		await user.type(input, "wrong-name");
		await user.keyboard("{Enter}");

		expect(onConfirm).not.toHaveBeenCalled();
		expect(onCancel).not.toHaveBeenCalled();
		expect(onAncestorKeyDown).not.toHaveBeenCalledWith(
			expect.objectContaining({ key: "Enter" }),
		);
		expect(input).toHaveFocus();
	});

	it("confirms on Enter without leaking it to ancestors when the name matches", async () => {
		const user = userEvent.setup();
		const { onConfirm, onAncestorKeyDown } = renderDialog();

		await user.type(
			screen.getByLabelText("Workspace name"),
			MockWorkspace.name,
		);
		await user.keyboard("{Enter}");

		expect(onConfirm).toHaveBeenCalledWith(false);
		expect(onAncestorKeyDown).not.toHaveBeenCalledWith(
			expect.objectContaining({ key: "Enter" }),
		);
	});
});
