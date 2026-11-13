import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	assignableRole,
	MockAuditorRole,
	MockAuthMethodsPasswordOnly,
	MockOwnerRole,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { CreateUserForm } from "./CreateUserForm";

const renderForm = (
	props: Partial<React.ComponentProps<typeof CreateUserForm>> = {},
) => {
	const onSubmit = vi.fn();
	render(
		<CreateUserForm
			isLoading={false}
			onSubmit={onSubmit}
			onCancel={vi.fn()}
			authMethods={MockAuthMethodsPasswordOnly}
			showOrganizations={false}
			serviceAccountsEnabled={false}
			{...props}
		/>,
	);
	return { onSubmit };
};

describe("CreateUserForm", () => {
	describe("without the service accounts feature", () => {
		it("lists the service account option as disabled", async () => {
			const user = userEvent.setup();
			renderForm();

			await user.click(await screen.findByTestId("login-type-input"));
			const option = await screen.findByRole("option", {
				name: /service account/i,
			});

			expect(option).toHaveAttribute("aria-disabled", "true");
		});

		it("does not create a service account", async () => {
			const user = userEvent.setup({ pointerEventsCheck: 0 });
			const { onSubmit } = renderForm();

			await user.click(await screen.findByTestId("login-type-input"));
			await user.click(
				await screen.findByRole("option", { name: /service account/i }),
			);
			await user.keyboard("{Escape}");
			await waitFor(() =>
				expect(screen.queryByRole("listbox")).not.toBeInTheDocument(),
			);

			await user.type(screen.getByLabelText(/username/i), "someuser");
			await user.type(screen.getByLabelText(/email/i), "someone@coder.com");
			await user.type(screen.getByTestId("password-input"), "SomePassword!");
			await user.click(screen.getByRole("button", { name: /save/i }));

			await waitFor(() => expect(onSubmit).toHaveBeenCalled());
			expect(onSubmit.mock.calls[0][0]).toMatchObject({
				login_type: "password",
				service_account: false,
			});
		});
	});

	it("submits the roles checked in the Roles group", async () => {
		const user = userEvent.setup();
		const { onSubmit } = renderForm({
			availableRoles: [
				assignableRole(MockOwnerRole, true),
				assignableRole(MockAuditorRole, true),
			],
		});

		const roles = screen.getByRole("group", { name: "Roles" });
		await user.click(within(roles).getByRole("checkbox", { name: /auditor/i }));
		await user.type(screen.getByLabelText(/username/i), "someuser");
		await user.type(screen.getByLabelText(/email/i), "someone@example.com");
		await user.type(screen.getByTestId("password-input"), "SomePassword!");
		await user.click(screen.getByRole("button", { name: /save/i }));

		await waitFor(() => expect(onSubmit).toHaveBeenCalled());
		expect(onSubmit.mock.calls[0][0].roles).toEqual(
			new Set([MockAuditorRole.name]),
		);
	});
});
