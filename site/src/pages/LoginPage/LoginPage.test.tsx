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

	it("allows retrying a failed sign-in without client navigation", async () => {
		const apiErrorMessage = "Something wrong happened";
		let loggedIn = false;
		const loginRequest = vi
			.fn()
			.mockImplementationOnce(() =>
				HttpResponse.json({ message: apiErrorMessage }, { status: 500 }),
			)
			.mockImplementation(() => {
				loggedIn = true;
				return HttpResponse.json({ session_token: "test-session-token" });
			});
		server.use(
			http.post("/api/v2/users/login", loginRequest),
			http.get("/api/v2/users/me", () =>
				loggedIn
					? HttpResponse.json(MockUserOwner)
					: HttpResponse.json({ message: "no user here" }, { status: 401 }),
			),
		);

		const { router } = renderWithRouter(
			createMemoryRouter(
				[
					{ path: "/login", element: <LoginPage /> },
					{ path: "/", element: null },
				],
				{ initialEntries: ["/login"] },
			),
		);
		await waitForLoaderToBeRemoved();
		// The live region exists before the error so screen readers announce it.
		const [errorRegion] = screen.getAllByRole("alert");
		expect(errorRegion).toBeEmptyDOMElement();
		const email = screen.getByLabelText(/Email/);
		const password = screen.getByLabelText(/Password/);
		await userEvent.type(email, "test@coder.com");
		await userEvent.type(password, "password");
		const signInButton = screen.getByRole("button", { name: "Sign in" });
		await userEvent.click(signInButton);
		await waitFor(() => expect(errorRegion).toHaveTextContent(apiErrorMessage));
		expect(locationHrefSpy).not.toHaveBeenCalled();

		await userEvent.click(signInButton);
		await waitFor(() => {
			expect(locationHrefSpy).toHaveBeenCalledExactlyOnceWith("/");
		});
		expect(loginRequest).toHaveBeenCalledTimes(2);
		expect(router.state.location.pathname).toBe("/login");
	});

	it("sets the document title", async () => {
		render(<LoginPage />);
		await waitForLoaderToBeRemoved();
		expect(document.title).toBe("Sign in to Coder");
	});

	it("associates validation errors with their inputs", async () => {
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

		const alertsBeforeSubmit = screen.getAllByRole("alert");
		const signInButton = await screen.findByText("Sign in");
		fireEvent.click(signInButton);

		// Then
		const emailError = await screen.findByText(
			"Please enter an email address.",
		);
		// Mounted before the error so screen readers announce it.
		expect(alertsBeforeSubmit).toContain(emailError);
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

		expect(passwordInput).toHaveAttribute("aria-invalid", "true");
		expect(passwordInput).toHaveAccessibleDescription(
			"Please enter a password.",
		);
	});

	it("does not sign in without a password", async () => {
		const loginRequest = vi.fn();
		server.use(
			http.post("/api/v2/users/login", () => {
				loginRequest();
				return HttpResponse.json({}, { status: 201 });
			}),
		);
		render(<LoginPage />);
		await waitForLoaderToBeRemoved();

		await userEvent.type(screen.getByLabelText(/Email/), "test@example.com");
		await userEvent.click(screen.getByRole("button", { name: "Sign in" }));

		await waitFor(() =>
			expect(screen.getByLabelText(/Password/)).toHaveAccessibleDescription(
				"Please enter a password.",
			),
		);
		expect(loginRequest).not.toHaveBeenCalled();
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

	it.each(["/", "/workspaces?filter=mine"])(
		"hard redirects to %s after password login without client navigation",
		async (redirectPath) => {
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

			const { router } = renderWithRouter(
				createMemoryRouter(
					[
						{ path: "/login", element: <LoginPage /> },
						{ path: "/", element: null },
						{ path: "/workspaces", element: null },
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
			await userEvent.click(screen.getByRole("button", { name: "Sign in" }));

			await waitFor(() => {
				expect(locationHrefSpy).toHaveBeenCalledExactlyOnceWith(redirectPath);
			});
			expect(router.state.location.pathname).toBe("/login");
		},
	);

	it("does not follow a protocol-relative redirect after password login (CDM-02-001)", async () => {
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

		// The decoded pathname is a protocol-relative URL, not a local path.
		const { router } = renderWithRouter(
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
				{
					initialEntries: ["/login?redirect=https://cure53.de/%2fcure53.de"],
				},
			),
		);

		await waitForLoaderToBeRemoved();

		await userEvent.type(screen.getByLabelText(/Email/), "test@coder.com");
		await userEvent.type(screen.getByLabelText(/Password/), "password");
		fireEvent.click(await screen.findByText("Sign in"));

		// The untrusted redirect must not escape the current origin.
		await waitFor(() => {
			expect(locationHrefSpy).toHaveBeenCalledWith("/");
		});
		expect(locationHrefSpy).not.toHaveBeenCalledWith(
			expect.stringContaining("//cure53.de"),
		);
		expect(router.state.location.pathname).toBe("/login");
	});

	it.each([
		{ redirectPath: "/workspaces?filter=mine", hardReload: false },
		{
			redirectPath:
				"/oauth2/authorize?client_id=xxx&response_type=code&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback",
			hardReload: true,
		},
	])(
		"redirects an already signed-in user to $redirectPath",
		async ({ redirectPath, hardReload }) => {
			server.use(
				http.get("/api/v2/users/me", () => {
					return HttpResponse.json(MockUserOwner);
				}),
			);

			const { router } = renderWithRouter(
				createMemoryRouter(
					[
						{ path: "/login", element: <LoginPage /> },
						{ path: "/workspaces", element: null },
					],
					{
						initialEntries: [
							`/login?redirect=${encodeURIComponent(redirectPath)}`,
						],
					},
				),
			);

			if (hardReload) {
				await waitFor(() => {
					expect(locationHrefSpy).toHaveBeenCalledWith(redirectPath);
				});
			} else {
				await waitFor(() => {
					expect(
						router.state.location.pathname + router.state.location.search,
					).toBe(redirectPath);
				});
				expect(locationHrefSpy).not.toHaveBeenCalled();
			}
		},
	);

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
		fireEvent.click(await screen.findByText("Sign in"));

		// Then - the full redirect path (including query params) must be
		// preserved for the OAuth2 authorization flow to complete.
		await waitFor(() => {
			expect(locationHrefSpy).toHaveBeenCalledWith(redirectPath);
		});
	});
});
