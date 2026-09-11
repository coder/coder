import { render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfirmDialog } from "./ConfirmDialog";

describe("ConfirmDialog", () => {
	it("does not close while confirming", async () => {
		const user = userEvent.setup();
		const onClose = vi.fn();
		render(
			<ConfirmDialog
				open
				title="Delete workspace"
				description="Are you sure?"
				confirmLoading
				onClose={onClose}
			/>,
		);

		await user.keyboard("{Escape}");

		expect(onClose).not.toHaveBeenCalled();
	});
});
