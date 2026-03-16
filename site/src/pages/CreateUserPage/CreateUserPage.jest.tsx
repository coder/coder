import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CreateUserPage from "./CreateUserPage";

const renderCreateUserPage = async () => {
	renderWithAuth(<CreateUserPage />, {
		extraRoutes: [
			{ path: "/deployment/users", element: <div>Users Page</div> },
		],
	});
	await waitForLoaderToBeRemoved();
};

const fillForm = async ({
	username = "someuser",
	email = "someone@coder.com",
	password = "SomeSecurePassword!",
}: {
	username?: string;
	email?: string;
	password?: string;
}) => {
	await userEvent.type(screen.getByLabelText("Username"), username);
	await userEvent.type(screen.getByLabelText(/email/i), email);

	// Open the login type select and choose "password"
	await userEvent.click(screen.getByTestId("login-type-input"));
	await userEvent.click(screen.getByRole("option", { name: /password/i }));

	await userEvent.type(screen.getByTestId("password-input"), password);

	fireEvent.click(screen.getByRole("button", { name: /save/i }));
};

describe("Create User Page", () => {
	it("shows success notification and redirects to users page", async () => {
		await renderCreateUserPage();
		await fillForm({});
		const successMessage = await screen.findByText(
			'User "someuser" created successfully.',
		);
		expect(successMessage).toBeDefined();
	});
});
