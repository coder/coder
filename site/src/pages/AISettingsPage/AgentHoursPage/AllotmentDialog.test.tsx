import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { mockApiError } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { AllotmentDialog, type AllotmentTarget } from "./AllotmentDialog";

type Submit = (target: AllotmentTarget, bps: number) => Promise<unknown>;

const renderDialog = (onSubmit: Submit) => {
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

const selectEngineering = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("combobox", { name: "Organization" }));
	await user.click(await screen.findByRole("option", { name: "Engineering" }));
};

describe("AllotmentDialog", () => {
	it("requires a chosen target and submits the percentage as basis points", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(async () => undefined);
		const { onClose } = renderDialog(onSubmit);

		await user.type(screen.getByRole("textbox", { name: "Allotment" }), "12.5");
		await user.click(screen.getByRole("button", { name: "Save" }));
		expect(onSubmit).not.toHaveBeenCalled();

		await selectEngineering(user);
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).toHaveBeenCalledWith(
			{ id: "org-1", name: "Engineering" },
			1250,
		);
		expect(onClose).toHaveBeenCalled();
	});

	it("does not submit more than the available share", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(async () => undefined);
		const { onClose } = renderDialog(onSubmit);

		await selectEngineering(user);
		await user.type(
			screen.getByRole("textbox", { name: "Allotment" }),
			"40.01",
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).not.toHaveBeenCalled();
		expect(onClose).not.toHaveBeenCalled();
	});

	it.each([
		["-0", "Enter a percentage above 0."],
		["abc", "Enter a number like 25 or 12.5."],
		["1.234", "Enter a number with at most two decimals."],
	])("rejects %s with a matching message", async (percent, message) => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(async () => undefined);
		renderDialog(onSubmit);

		await selectEngineering(user);
		await user.type(
			screen.getByRole("textbox", { name: "Allotment" }),
			percent,
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		await screen.findByText(message);
		expect(onSubmit).not.toHaveBeenCalled();
	});

	it("stays open after a server conflict so the save can be retried", async () => {
		const user = userEvent.setup();
		const onSubmit = vi
			.fn<Submit>()
			.mockRejectedValueOnce(
				mockApiError({
					message: "Agent Hours allotments cannot exceed 100% in total.",
				}),
			)
			.mockResolvedValueOnce(undefined);
		const { onClose } = renderDialog(onSubmit);

		await selectEngineering(user);
		await user.type(screen.getByRole("textbox", { name: "Allotment" }), "30");
		await user.click(screen.getByRole("button", { name: "Save" }));
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => expect(onClose).toHaveBeenCalled());
		expect(onSubmit).toHaveBeenCalledTimes(2);
		expect(onClose).toHaveBeenCalledTimes(1);
	});
});
