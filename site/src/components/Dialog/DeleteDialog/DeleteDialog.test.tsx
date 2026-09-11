import { render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DeleteDialog } from "./DeleteDialog";

describe("DeleteDialog", () => {
	it("does not cancel while deleting", async () => {
		const user = userEvent.setup();
		const onCancel = vi.fn();
		render(
			<DeleteDialog
				isOpen
				entity="workspace"
				name="my-workspace"
				confirmLoading
				onCancel={onCancel}
				onConfirm={vi.fn()}
			/>,
		);

		await user.keyboard("{Escape}");

		expect(onCancel).not.toHaveBeenCalled();
	});
});
