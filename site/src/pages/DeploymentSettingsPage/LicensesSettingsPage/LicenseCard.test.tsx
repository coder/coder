import { MockLicenseResponse } from "testHelpers/entities";
import { render } from "testHelpers/renderHelpers";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import dayjs from "dayjs";
import { LicenseCard } from "./LicenseCard";

const openRemoveDialog = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(
		screen.getByRole("button", { name: /show license actions/i }),
	);
	await user.click(await screen.findByRole("menuitem", { name: /remove/i }));
};

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

	it("does not show AI add-on card when only AI feature limit is present", () => {
		const license = {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
			},
		};

		render(
			<LicenseCard
				license={license}
				aiGovernanceUserFeature={{
					enabled: true,
					entitlement: "entitled",
					actual: 100,
				}}
				userLimitActual={1}
				userLimitLimit={10}
				onRemove={() => null}
				isRemoving={false}
			/>,
		);

		expect(screen.queryByText("Add-ons")).not.toBeInTheDocument();
		expect(screen.queryByText("AI governance")).not.toBeInTheDocument();
	});

	it("shows AI add-on card when explicit AI add-on is present", async () => {
		const license = {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		};

		render(
			<LicenseCard
				license={license}
				aiGovernanceUserFeature={{
					enabled: true,
					entitlement: "entitled",
					actual: 100,
				}}
				userLimitActual={1}
				userLimitLimit={10}
				onRemove={() => null}
				isRemoving={false}
			/>,
		);

		await screen.findByText("Add-ons");
		await screen.findByText("AI governance");
	});

	it("shows add-on exceeded during grace period after license expiry", async () => {
		const license = {
			...MockLicenseResponse[1],
			claims: {
				...MockLicenseResponse[1].claims,
				license_expires: dayjs().subtract(1, "day").unix(),
				features: {
					...MockLicenseResponse[1].claims.features,
					ai_governance_user_limit: 1000,
				},
				addons: ["ai_governance"],
			},
		};

		render(
			<LicenseCard
				license={license}
				aiGovernanceUserFeature={{
					enabled: true,
					entitlement: "grace_period",
					actual: 1200,
					limit: 1000,
				}}
				userLimitActual={1}
				userLimitLimit={10}
				onRemove={() => null}
				isRemoving={false}
			/>,
		);

		await screen.findByText("Add-on exceeded");
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
