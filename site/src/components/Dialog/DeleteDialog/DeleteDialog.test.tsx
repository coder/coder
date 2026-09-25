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

// The parent owns `isOpen`, as real callers do. Some close the dialog after
// confirming (including after a failed delete, as the organization page
// does), others keep it open so the user can retry.
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
			"\u201cwrong-name\u201d does not match the name of this workspace",
		);
	});

	it("clears the submitted error when the user edits the name", async () => {
		const user = userEvent.setup();
		const { input } = renderDialog();

		await user.type(input, "wrong-name{Enter}x");

		expect(input).toHaveAttribute("aria-invalid", "false");
		expect(screen.getByRole("alert")).toBeEmptyDOMElement();
	});

	it("confirms on Enter when the name matches", async () => {
		const user = userEvent.setup();
		const { onConfirm, input } = renderDialog();

		await user.type(input, "my-workspace{Enter}");

		expect(onConfirm).toHaveBeenCalledTimes(1);
	});

	it("keeps the typed name while the dialog stays open after confirming, so a failed delete can be retried", async () => {
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
