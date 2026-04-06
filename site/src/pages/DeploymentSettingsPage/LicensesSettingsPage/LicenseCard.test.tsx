import { screen, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { MockLicenseResponse } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { LicenseCard } from "./LicenseCard";

const openRemoveDialog = async (user: UserEvent) => {
	await user.click(
		screen.getByRole("button", { name: /show license actions/i }),
	);
	await user.click(await screen.findByRole("menuitem", { name: /remove/i }));
};

describe("LicenseCard", () => {
	it("shows expired removal message for expired licenses", async () => {
		const user = userEvent.setup();
		render(
			<LicenseCard
				license={MockLicenseResponse[3]}
				userLimitActual={1}
				userLimitLimit={10}
				onRemove={() => null}
				isRemoving={false}
			/>,
		);

		await openRemoveDialog(user);

		const dialog = await screen.findByTestId("dialog");
		expect(dialog).toHaveTextContent(/This license has already expired/);
	});

	it("shows disabling features warning for active licenses", async () => {
		const user = userEvent.setup();
		render(
			<LicenseCard
				license={MockLicenseResponse[0]}
				userLimitActual={1}
				userLimitLimit={10}
				onRemove={() => null}
				isRemoving={false}
			/>,
		);

		await openRemoveDialog(user);

		await screen.findByText(
			/Removing this license will disable all Premium features/,
		);
	});

	it("requires typing the license ID before allowing removal", async () => {
		const user = userEvent.setup();
		const onRemove = vi.fn();
		const license = MockLicenseResponse[0];

		render(
			<LicenseCard
				license={license}
				userLimitActual={1}
				userLimitLimit={10}
				onRemove={onRemove}
				isRemoving={false}
			/>,
		);

		await openRemoveDialog(user);

		const dialog = await screen.findByTestId("dialog");
		const dialogScope = within(dialog);
		const confirmButton = dialogScope.getByRole("button", { name: "Remove" });
		expect(confirmButton).toBeDisabled();

		const confirmationInput = dialogScope.getByTestId(
			"delete-dialog-name-confirmation",
		);
		await user.type(confirmationInput, "wrong");
		expect(confirmButton).toBeDisabled();

		await user.clear(confirmationInput);
		await user.type(confirmationInput, String(license.id));
		expect(confirmButton).toBeEnabled();

		await user.click(confirmButton);
		expect(onRemove).toHaveBeenCalledWith(license.id);
	});
});
