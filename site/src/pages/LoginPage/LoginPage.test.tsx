import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { createMemoryRouter } from "react-router";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	render,
	renderWithRouter,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import LoginPage from "./LoginPage";

describe("LoginPage", () => {
	// Capture original values before any test stubs take effect.
	const origLocationOrigin = location.origin;
	const origLocationHref = location.href;
	const locationHrefSpy = vi.fn();

	beforeEach(() => {
		server.use(
			// Appear logged out
			http.get("/api/v2/users/me", () => {
				return HttpResponse.json({ message: "no user here" }, { status: 401 });
			}),
		);
		// Stub the location global so tests can intercept server-side redirects
		// without actually navigating away.
		vi.stubGlobal("location", {
			origin: origLocationOrigin,
			get href() {
				return origLocationHref;
			},
			set href(url: string) {
				locationHrefSpy(url);
			},
		});
	});

	afterEach(() => {
		vi.unstubAllGlobals();
		locationHrefSpy.mockReset();
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
		const email = screen.getByLabelText(/Email/);
		const password = screen.getByLabelText(/Password/);
		await userEvent.type(email, "test@coder.com");
		await userEvent.type(password, "password");
		// Click sign-in
		const signInButton = await screen.findByText("Sign In");
		fireEvent.click(signInButton);

		// Then
		const errorMessage = await screen.findByText(apiErrorMessage);
		expect(errorMessage).toBeDefined();
	});

	it("associates email validation errors with the email input", async () => {
		// When
		render(<LoginPage />);
		await waitForLoaderToBeRemoved();

		const emailInput = screen.getByLabelText(/Email/);
		const passwordInput = screen.getByLabelText(/Password/);
		expect(emailInput).not.toHaveAttribute("aria-invalid", "true");
		expect(emailInput).not.toHaveAttribute(
			"aria-describedby",
			"signin-email-error",
		);
		expect(passwordInput).not.toHaveAttribute("aria-invalid", "true");
		expect(passwordInput).not.toHaveAttribute(
			"aria-describedby",
			"signin-password-error",
		);

		const signInButton = await screen.findByText("Sign In");
		fireEvent.click(signInButton);

		// Then
		const emailError = await screen.findByText(
			"Please enter an email address.",
		);
		expect(emailInput).toHaveAttribute("aria-invalid", "true");
		expect(emailInput).toHaveAttribute(
			"aria-describedby",
			"signin-email-error",
		);

		const emailErrorElement = document.getElementById("signin-email-error");
		expect(emailErrorElement).toBe(emailError);
		expect(emailErrorElement).toHaveTextContent(
			"Please enter an email address.",
		);

		expect(passwordInput).not.toHaveAttribute("aria-invalid", "true");
		expect(passwordInput).not.toHaveAttribute(
			"aria-describedby",
			"signin-password-error",
		);
		expect(document.getElementById("signin-password-error")).toBeNull();
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

	it("navigates to the home page after successful password login", async () => {
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

		// When
		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/login",
						element: <LoginPage />,
					},
					{
						path: "/",
						element: <h1>Home</h1>,
					},
				],
				{ initialEntries: ["/login"] },
			),
		);

		await waitForLoaderToBeRemoved();

		await userEvent.type(screen.getByLabelText(/Email/), "test@coder.com");
		await userEvent.type(screen.getByLabelText(/Password/), "password");
		fireEvent.click(await screen.findByText("Sign In"));

		// Then - the component uses React Router navigation for standard
		// redirects so the new session cookie is picked up by the next route.
		await screen.findByText("Home");
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

		// Then - the full redirect path (including query params) must be
		// preserved for the OAuth2 authorization flow to complete.
		await waitFor(() => {
			expect(locationHrefSpy).toHaveBeenCalledWith(redirectPath);
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

		await userEvent.type(screen.getByLabelText(/Email/), "test@coder.com");
		await userEvent.type(screen.getByLabelText(/Password/), "password");
		fireEvent.click(await screen.findByText("Sign In"));

		// Then - the full redirect path (including query params) must be
		// preserved for the OAuth2 authorization flow to complete.
		await waitFor(() => {
			expect(locationHrefSpy).toHaveBeenCalledWith(redirectPath);
		});
	});
});
