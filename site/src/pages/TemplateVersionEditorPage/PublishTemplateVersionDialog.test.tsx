import { renderComponent } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("components/Dialogs/ConfirmDialog/ConfirmDialog", () => ({
	ConfirmDialog: ({ onClose }: { onClose: () => void }) => (
		<button type="button" onClick={onClose}>
			Request close
		</button>
	),
}));

import { PublishTemplateVersionDialog } from "./PublishTemplateVersionDialog";

describe("PublishTemplateVersionDialog", () => {
	it("ignores close requests while publishing", async () => {
		const user = userEvent.setup();
		const onClose = vi.fn();

		renderComponent(
			<PublishTemplateVersionDialog
				defaultName="test-version"
				isPublishing
				onClose={onClose}
				onConfirm={vi.fn()}
				open
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Request close" }));

		expect(onClose).not.toHaveBeenCalled();
	});

	it("allows closing when publishing is idle", async () => {
		const user = userEvent.setup();
		const onClose = vi.fn();

		renderComponent(
			<PublishTemplateVersionDialog
				defaultName="test-version"
				isPublishing={false}
				onClose={onClose}
				onConfirm={vi.fn()}
				open
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Request close" }));

		expect(onClose).toHaveBeenCalledTimes(1);
	});
});
