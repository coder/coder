import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type FC, useState } from "react";
import { render } from "#/testHelpers/renderHelpers";
import { DeleteDialog } from "./DeleteDialog";

type HarnessProps = {
	onConfirm: () => void;
	onCancel: () => void;
	closeOnConfirm: boolean;
};

// Callers either close the dialog after confirming or keep it open to retry.
const Harness: FC<HarnessProps> = ({ onConfirm, onCancel, closeOnConfirm }) => {
	const [isOpen, setIsOpen] = useState(true);
	return (
		<>
			<button type="button" onClick={() => setIsOpen(true)}>
				Reopen
			</button>
			<DeleteDialog
				isOpen={isOpen}
				onConfirm={() => {
					if (closeOnConfirm) {
						setIsOpen(false);
					}
					onConfirm();
				}}
				onCancel={() => {
					setIsOpen(false);
					onCancel();
				}}
				entity="workspace"
				name="my-workspace"
			/>
		</>
	);
};

const renderDialog = ({ closeOnConfirm = false } = {}) => {
	const onConfirm = vi.fn();
	const onCancel = vi.fn();

	render(
		<Harness
			onConfirm={onConfirm}
			onCancel={onCancel}
			closeOnConfirm={closeOnConfirm}
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
		expect(input).toHaveAccessibleDescription(
			"“wrong-name” does not match the name of this workspace",
		);
	});

	it("clears the submitted error when the user edits the name", async () => {
		const user = userEvent.setup();
		const { input } = renderDialog();

		await user.type(input, "wrong-name{Enter}x");

		expect(input).toHaveAttribute("aria-invalid", "false");
		expect(screen.queryByRole("alert")).toBeNull();
	});

	it("confirms on Enter and keeps the typed name while the dialog stays open", async () => {
		const user = userEvent.setup();
		const { onConfirm, input } = renderDialog();

		await user.type(input, "my-workspace{Enter}");

		expect(onConfirm).toHaveBeenCalledTimes(1);
		expect(input).toHaveValue("my-workspace");
	});

	it("starts empty when reopened after the parent closes it", async () => {
		const user = userEvent.setup();
		const { onConfirm, input } = renderDialog({ closeOnConfirm: true });

		await user.type(input, "my-workspace{Enter}");
		expect(onConfirm).toHaveBeenCalledTimes(1);

		await user.click(screen.getByRole("button", { name: "Reopen" }));

		expect(
			await screen.findByLabelText("Name of the workspace to delete"),
		).toHaveValue("");
		expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled();
	});
});
