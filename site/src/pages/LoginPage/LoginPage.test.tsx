import { fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { createMemoryRouter } from "react-router-dom";
import {
	render,
	renderWithRouter,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { Language } from "./Language";
import LoginPage from "./LoginPage";

describe("LoginPage", () => {
	beforeEach(() => {
		server.use(
			// Appear logged out
			http.get("/api/v2/users/me", () => {
				return HttpResponse.json({ message: "no user here" }, { status: 401 });
			}),
		);
	});

	it("shows an error message if SignIn fails", async () => {
		// Given
		const apiErrorMessage = "Something wrong happened";
		server.use(
			// Make login fail
			http.post("/api/v2/users/login", async () => {
				return HttpResponse.json({ message: apiErrorMessage }, { status: 500 });
			}),
		);

		// When
		render(<LoginPage />);
		await waitForLoaderToBeRemoved();
		const email = screen.getByLabelText(Language.emailLabel);
		const password = screen.getByLabelText(Language.passwordLabel);
		await userEvent.type(email, "test@coder.com");
		await userEvent.type(password, "password");
		// Click sign-in
		const signInButton = await screen.findByText(Language.passwordSignIn);
		fireEvent.click(signInButton);

		// Then
		const errorMessage = await screen.findByText(apiErrorMessage);
		expect(errorMessage).toBeDefined();
	});

	it("redirects to the setup page if there is no first user", async () => {
		// Given
		server.use(
			// No first user
			http.get("/api/v2/users/first", () => {
				return new HttpResponse(null, { status: 404 });
			}),
		);

		// When
		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/login",
						element: <LoginPage />,
					},
					{
						path: "/setup",
						element: <h1>Setup</h1>,
					},
				],
				{ initialEntries: ["/login"] },
			),
		);

		// Then
		await screen.findByText("Setup");
	});
});
