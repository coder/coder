import { MockLicenseResponse } from "testHelpers/entities";
import { render } from "testHelpers/renderHelpers";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LicenseCard } from "./LicenseCard";

describe("LicenseCard", () => {
	it("renders (smoke test)", async () => {
		// When
		render(
			<LicenseCard
				license={MockLicenseResponse[0]}
				userLimitActual={1}
				userLimitLimit={10}
				onRemove={() => null}
				isRemoving={false}
			/>,
		);

		// Then
		await screen.findByText("#1");
		await screen.findByText("1 / 10");
		await screen.findByText("Enterprise");
	});

	it("renders userLimit as unlimited if there is not user limit", async () => {
		// When
		render(
			<LicenseCard
				license={MockLicenseResponse[0]}
				userLimitActual={1}
				userLimitLimit={undefined}
				onRemove={() => null}
				isRemoving={false}
			/>,
		);

		// Then
		await screen.findByText("#1");
		await screen.findByText("1 / Unlimited");
		await screen.findByText("Enterprise");
	});

	it("renders license's user_limit when it is available instead of using the default", async () => {
		const licenseUserLimit = 3;
		const license = {
			...MockLicenseResponse[0],
			claims: {
				...MockLicenseResponse[0].claims,
				features: {
					...MockLicenseResponse[0].claims.features,
					user_limit: licenseUserLimit,
				},
			},
		};

		// When
		render(
			<LicenseCard
				license={license}
				userLimitActual={1}
				userLimitLimit={100} // This should not be used
				onRemove={() => null}
				isRemoving={false}
			/>,
		);

		// Then
		await screen.findByText("1 / 3");
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

		await user.click(screen.getByRole("button", { name: /remove/i }));

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
