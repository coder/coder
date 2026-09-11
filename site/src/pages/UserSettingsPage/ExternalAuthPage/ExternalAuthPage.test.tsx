import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type {
	ExternalAuth,
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

const providerResponse = (authenticated: boolean): ExternalAuth => ({
	authenticated,
	device: false,
	display_name: "GitHub",
	supports_revocation: false,
	user: null,
	app_installable: false,
	installations: [],
	app_install_url: "",
});

const failedLink = {
	...MockGithubAuthLink,
	authenticated: false,
	validate_error: validateError,
};

describe("ExternalAuthPage", () => {
	// Regression test: the list query is the only source of validate_error, and
	// validateExternalAuth used to update only the per-provider query. The stale
	// error then sat next to the fresh state until a full page reload.
	it("refetches the list after a successful Test Validate so a stale error clears", async () => {
		let validated = false;
		let listRequests = 0;

		server.use(
			http.get("/api/v2/external-auth", () => {
				listRequests++;
				return HttpResponse.json<ListUserExternalAuthResponse>({
					providers: [MockGithubExternalProvider],
					links: [validated ? MockGithubAuthLink : failedLink],
				});
			}),
			http.get("/api/v2/external-auth/github", () => {
				// The first request is the row's own provider query. The next one
				// is the "Test Validate" mutation, which succeeds.
				const response = providerResponse(validated);
				validated = true;
				return HttpResponse.json(response);
			}),
		);

		renderWithAuth(<ExternalAuthPage />);
		await screen.findByText(validateError, { exact: false });

		const user = userEvent.setup();
		await user.click(await screen.findByRole("button", { name: "Open menu" }));
		await user.click(
			await screen.findByRole("menuitem", { name: /Test Validate/ }),
		);

		await waitFor(() => {
			expect(
				screen.queryByText(validateError, { exact: false }),
			).not.toBeInTheDocument();
		});
		// The list endpoint is hit again by the post-validate invalidation, so
		// the displayed link reflects current state.
		expect(listRequests).toBeGreaterThan(1);
	});

	it("does not render a stale error next to an authenticated row", async () => {
		server.use(
			http.get("/api/v2/external-auth", () => {
				return HttpResponse.json<ListUserExternalAuthResponse>({
					providers: [MockGithubExternalProvider],
					links: [failedLink],
				});
			}),
			// The per-provider query validates live and disagrees with the list.
			http.get("/api/v2/external-auth/github", () => {
				return HttpResponse.json(providerResponse(true));
			}),
		);

		renderWithAuth(<ExternalAuthPage />);

		await screen.findByRole("button", { name: "Authenticated" });
		expect(
			screen.queryByText(validateError, { exact: false }),
		).not.toBeInTheDocument();
	});
});
