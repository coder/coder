import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type FC, useState } from "react";
import type { Workspace } from "#/api/typesGenerated";
import { MockFailedWorkspace, MockWorkspace } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";

type HarnessProps = {
	workspace: Workspace;
	canDeleteFailedWorkspace: boolean;
	onConfirm: (orphan: boolean | undefined) => void;
	onCancel: () => void;
};

// Mirrors WorkspaceMoreActions: the dialog stays mounted and the parent
// toggles `isOpen`, closing it on both cancel and confirm.
const Harness: FC<HarnessProps> = ({ onConfirm, onCancel, ...props }) => {
	const [isOpen, setIsOpen] = useState(true);
	return (
		<>
			<button type="button" onClick={() => setIsOpen(true)}>
				Reopen
			</button>
			<WorkspaceDeleteDialog
				{...props}
				isOpen={isOpen}
				onCancel={() => {
					setIsOpen(false);
					onCancel();
				}}
				onConfirm={(orphan) => {
					setIsOpen(false);
					onConfirm(orphan);
				}}
			/>
		</>
	);
};

const renderDialog = (
	workspace: Workspace = MockWorkspace,
	canDeleteFailedWorkspace = false,
) => {
	const onConfirm = vi.fn();
	const onCancel = vi.fn();

	render(
		<Harness
			workspace={workspace}
			canDeleteFailedWorkspace={canDeleteFailedWorkspace}
			onConfirm={onConfirm}
			onCancel={onCancel}
		/>,
	);

	return { onConfirm, onCancel };
};

const getInput = () => screen.getByLabelText("Workspace name");

const reopen = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("button", { name: "Reopen" }));
	return screen.findByLabelText("Workspace name");
};

describe("WorkspaceDeleteDialog", () => {
	it("marks the input invalid instead of confirming when Enter is pressed with a wrong name", async () => {
		const user = userEvent.setup();
		const { onConfirm, onCancel } = renderDialog();

		const input = getInput();
		await user.type(input, "wrong name{Enter}");

		expect(onConfirm).not.toHaveBeenCalled();
		expect(onCancel).not.toHaveBeenCalled();
		expect(input).toHaveFocus();
		expect(input).toHaveAttribute("aria-invalid", "true");
		expect(input).toHaveAccessibleDescription(
			"“wrong name” does not match the name of this workspace",
		);
	});

	it("clears the submitted error when the user edits the name", async () => {
		const user = userEvent.setup();
		renderDialog();

		const input = getInput();
		await user.type(input, "wrong name{Enter}x");

		expect(input).toHaveAttribute("aria-invalid", "false");
		expect(screen.queryByRole("alert")).toBeNull();
	});

	it("confirms on Enter when the name matches", async () => {
		const user = userEvent.setup();
		const { onConfirm } = renderDialog();

		await user.type(getInput(), `${MockWorkspace.name}{Enter}`);

		expect(onConfirm).toHaveBeenCalledWith(false);
	});

	it("starts empty when reopened after cancelling with the correct name typed", async () => {
		const user = userEvent.setup();
		const { onCancel } = renderDialog();

		await user.type(getInput(), MockWorkspace.name);
		await user.keyboard("{Escape}");
		expect(onCancel).toHaveBeenCalledTimes(1);

		const input = await reopen(user);
		expect(input).toHaveValue("");
		expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled();
	});

	it("starts empty when reopened after confirming", async () => {
		const user = userEvent.setup();
		const { onConfirm } = renderDialog();

		await user.type(getInput(), `${MockWorkspace.name}{Enter}`);
		expect(onConfirm).toHaveBeenCalledTimes(1);

		const input = await reopen(user);
		expect(input).toHaveValue("");
		expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled();
	});

	it("unchecks Orphan Resources when reopened", async () => {
		const user = userEvent.setup();
		renderDialog(MockFailedWorkspace, true);

		await user.click(
			screen.getByRole("checkbox", { name: /Orphan Resources/ }),
		);
		await user.keyboard("{Escape}");
		await reopen(user);

		expect(
			screen.getByRole("checkbox", { name: /Orphan Resources/ }),
		).not.toBeChecked();
	});
});
