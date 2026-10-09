import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { mockApiError } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { AllotmentDialog } from "./AllotmentDialog";

const renderDialog = (
	onSubmit: (targetId: string, bps: number) => Promise<unknown>,
) => {
	const onClose = vi.fn();
	render(
		<AllotmentDialog
			onClose={onClose}
			entity="organization"
			candidates={[{ id: "org-1", name: "Engineering" }]}
			availableBps={4000}
			poolHours={1000}
			onSubmit={onSubmit}
		/>,
	);
	return { onClose };
};

describe("AllotmentDialog", () => {
	it("submits the percentage as basis points", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(async () => undefined);
		const { onClose } = renderDialog(onSubmit);

		await user.type(screen.getByRole("textbox", { name: "Allotment" }), "12.5");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).toHaveBeenCalledWith("org-1", 1250);
		expect(onClose).toHaveBeenCalled();
	});

	it("does not submit more than the available share", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(async () => undefined);
		const { onClose } = renderDialog(onSubmit);

		await user.type(
			screen.getByRole("textbox", { name: "Allotment" }),
			"40.01",
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).not.toHaveBeenCalled();
		expect(onClose).not.toHaveBeenCalled();
	});

	it("keeps the dialog open with the server's conflict message", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(() =>
			Promise.reject(
				mockApiError({
					message: "Agent Hours allotments cannot exceed 100% in total.",
					detail: "Only 10% is unallotted.",
				}),
			),
		);
		const { onClose } = renderDialog(onSubmit);

		await user.type(screen.getByRole("textbox", { name: "Allotment" }), "30");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await screen.findByText(
			"Agent Hours allotments cannot exceed 100% in total.",
		);
		expect(onClose).not.toHaveBeenCalled();
	});
});
