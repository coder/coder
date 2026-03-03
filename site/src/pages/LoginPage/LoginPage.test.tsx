import { MockUserOwner } from "testHelpers/entities";
import {
	render,
	renderWithRouter,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { createMemoryRouter } from "react-router";
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
		const email = screen.getByLabelText(new RegExp(Language.emailLabel));
		const password = screen.getByLabelText(new RegExp(Language.passwordLabel));
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

	it("redirects to /oauth2/authorize via server-side redirect when signed in", async () => {
		// Given - user is signed in
		server.use(
			http.get("/api/v2/users/me", () => {
				return HttpResponse.json(MockUserOwner);
			}),
		);

		const redirectPath =
			"/oauth2/authorize?client_id=xxx&response_type=code&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback";

		// Spy on window.location.href assignment
		const locationHrefSpy = vi.fn();
		const originalLocation = window.location;
		Object.defineProperty(window, "location", {
			configurable: true,
			value: {
				...originalLocation,
				origin: originalLocation.origin,
				set href(url: string) {
					locationHrefSpy(url);
				},
				get href() {
					return originalLocation.href;
				},
			},
		});

		// When
		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/login",
						element: <LoginPage />,
					},
				],
				{
					initialEntries: [
						`/login?redirect=${encodeURIComponent(redirectPath)}`,
					],
				},
			),
		);

		// Then - it should perform a server-side redirect, not a React navigate
		await waitFor(() => {
			expect(locationHrefSpy).toHaveBeenCalledWith(
				expect.stringContaining("/oauth2/authorize"),
			);
		});

		// Cleanup
		Object.defineProperty(window, "location", {
			configurable: true,
			value: originalLocation,
		});
	});

	it("redirects to /oauth2/authorize after successful login when not already signed in", async () => {
		// Given - user is NOT signed in
		let loggedIn = false;
		server.use(
			http.get("/api/v2/users/me", () => {
				if (!loggedIn) {
					return HttpResponse.json(
						{ message: "no user here" },
						{ status: 401 },
					);
				}
				return HttpResponse.json(MockUserOwner);
			}),
			http.post("/api/v2/users/login", () => {
				loggedIn = true;
				return HttpResponse.json({
					session_token: "test-session-token",
				});
			}),
		);

		const redirectPath =
			"/oauth2/authorize?client_id=xxx&response_type=code&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback";

		// Spy on window.location.href
		const originalLocation = window.location;
		const locationHrefSpy = vi.fn();

		Object.defineProperty(window, "location", {
			configurable: true,
			value: {
				...originalLocation,
				origin: originalLocation.origin,
				set href(url: string) {
					locationHrefSpy(url);
				},
				get href() {
					return originalLocation.href;
				},
			},
		});

		// When
		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/login",
						element: <LoginPage />,
					},
				],
				{
					initialEntries: [
						`/login?redirect=${encodeURIComponent(redirectPath)}`,
					],
				},
			),
		);

		await waitForLoaderToBeRemoved();

		const email = screen.getByLabelText(new RegExp(Language.emailLabel));
		const password = screen.getByLabelText(new RegExp(Language.passwordLabel));

		await userEvent.type(email, "test@coder.com");
		await userEvent.type(password, "password");

		const signInButton = await screen.findByText(Language.passwordSignIn);
		fireEvent.click(signInButton);

		// Then - it should hard redirect to OAuth endpoint
		await waitFor(() => {
			expect(locationHrefSpy).toHaveBeenCalledWith(
				expect.stringContaining("/oauth2/authorize"),
			);
		});

		// Cleanup
		Object.defineProperty(window, "location", {
			configurable: true,
			value: originalLocation,
		});
	});
});
