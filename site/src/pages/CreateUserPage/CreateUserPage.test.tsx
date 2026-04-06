import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import CreateUserPage from "./CreateUserPage";

const fillForm = async ({
	username = "someuser",
	email = "someone@coder.com",
	password = "SomeSecurePassword!",
}: {
	username?: string;
	email?: string;
	password?: string;
} = {}) => {
	await userEvent.type(screen.getByLabelText("Username"), username);
	await userEvent.type(screen.getByLabelText(/email/i), email);

	await userEvent.click(screen.getByTestId("login-type-input"));
	await userEvent.click(screen.getByRole("option", { name: /password/i }));

	await userEvent.type(screen.getByTestId("password-input"), password);

	await userEvent.click(screen.getByRole("button", { name: /save/i }));
};

describe("CreateUserPage", () => {
	it("shows a success notification and redirects to the users page on submit", async () => {
		renderWithAuth(<CreateUserPage />, {
			extraRoutes: [
				{ path: "/deployment/users", element: <div>Users Page</div> },
			],
		});
		await waitForLoaderToBeRemoved();
		await fillForm();
		await expect(
			screen.findByText('User "someuser" created successfully.'),
		).resolves.toBeInTheDocument();
	});

	it("shows an error alert when user creation fails", async () => {
		server.use(
			http.post("/api/v2/users", () => {
				return HttpResponse.json(
					{ message: "Username already in use." },
					{ status: 400 },
				);
			}),
		);

		renderWithAuth(<CreateUserPage />, {
			extraRoutes: [
				{ path: "/deployment/users", element: <div>Users Page</div> },
			],
		});
		await waitForLoaderToBeRemoved();
		await fillForm();
		await expect(
			screen.findByText("Username already in use."),
		).resolves.toBeInTheDocument();
	});
});
