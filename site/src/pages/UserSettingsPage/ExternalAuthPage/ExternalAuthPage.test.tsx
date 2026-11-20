import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type {
	ExternalAuth,
	ExternalAuthLink,
	ListUserExternalAuthResponse,
} from "#/api/typesGenerated";
import {
	MockGithubAuthLink,
	MockGithubExternalProvider,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import ExternalAuthPage from "./ExternalAuthPage";

const validateError = "token failed to validate";

const mockFailedGithubAuthLink: ExternalAuthLink = {
	...MockGithubAuthLink,
	authenticated: false,
	validate_error: validateError,
};

const mockGithubExternalAuth: ExternalAuth = {
	authenticated: true,
	device: false,
	display_name: MockGithubExternalProvider.display_name,
	supports_revocation: false,
	user: null,
	app_installable: false,
	installations: [],
	app_install_url: "",
};

const mockUnauthenticatedGithubExternalAuth: ExternalAuth = {
	...mockGithubExternalAuth,
	authenticated: false,
};

// Simulates a backend whose token validity can change between requests. The
// list endpoint reports validate_error from its own validation pass, like
// coderd does.
const mockExternalAuthBackend = (initiallyValid: boolean) => {
	const backend = { tokenValid: initiallyValid, listRequests: 0 };
	server.use(
		http.get("/api/v2/external-auth", () => {
			backend.listRequests++;
			return HttpResponse.json<ListUserExternalAuthResponse>({
				providers: [MockGithubExternalProvider],
				links: [
					backend.tokenValid ? MockGithubAuthLink : mockFailedGithubAuthLink,
				],
			});
		}),
		http.get(`/api/v2/external-auth/${MockGithubExternalProvider.id}`, () =>
			HttpResponse.json(
				backend.tokenValid
					? mockGithubExternalAuth
					: mockUnauthenticatedGithubExternalAuth,
			),
		),
	);
	return backend;
};

describe("ExternalAuthPage", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("refetches the list after a successful Test Validate so the stale error clears", async () => {
		const backend = mockExternalAuthBackend(false);
		renderWithAuth(<ExternalAuthPage />);
		await screen.findByText(validateError);
		expect(backend.listRequests).toBe(1);

		backend.tokenValid = true;
		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: "Open menu" }));
		await user.click(
			await screen.findByRole("menuitem", { name: /Test Validate/ }),
		);

		await waitFor(() => expect(backend.listRequests).toBe(2));
		await waitFor(() => expect(screen.queryByText(validateError)).toBeNull());
	});

	it("does not show the list's validate_error when the provider reports authenticated", async () => {
		server.use(
			http.get("/api/v2/external-auth", () =>
				HttpResponse.json<ListUserExternalAuthResponse>({
					providers: [MockGithubExternalProvider],
					links: [mockFailedGithubAuthLink],
				}),
			),
			http.get(`/api/v2/external-auth/${MockGithubExternalProvider.id}`, () =>
				HttpResponse.json(mockGithubExternalAuth),
			),
		);
		renderWithAuth(<ExternalAuthPage />);

		await screen.findByRole("button", { name: "Authenticated" });
		expect(screen.queryByText(validateError)).toBeNull();
	});

	it("refetches the list when polling observes a completed login", async () => {
		const openWindow = vi.fn();
		vi.stubGlobal("open", openWindow);
		const backend = mockExternalAuthBackend(false);
		renderWithAuth(<ExternalAuthPage />);

		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", { name: "Click to Login" }),
		);
		expect(openWindow).toHaveBeenCalledWith(
			`/external-auth/${MockGithubExternalProvider.id}`,
			"_blank",
			"width=900,height=600",
		);
		backend.tokenValid = true;

		await waitFor(() => expect(backend.listRequests).toBe(2), {
			timeout: 3_000,
		});
		await waitFor(() => expect(screen.queryByText(validateError)).toBeNull());
	});
});
