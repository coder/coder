import { screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ExternalAuth, ExternalAuthDevice } from "#/api/typesGenerated";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import ExternalAuthPage from "./ExternalAuthPage";

const provider = "github";

const deviceResponse: ExternalAuthDevice = {
	device_code: "device-code",
	user_code: "1234-5678",
	verification_uri: "https://github.com/login/device",
	expires_in: 900,
	interval: 0,
};

const baseProvider: ExternalAuth = {
	authenticated: false,
	device: true,
	display_name: "GitHub",
	supports_revocation: false,
	user: null,
	app_installable: false,
	installations: [],
	app_install_url: "",
};

const renderPage = () =>
	renderWithAuth(<ExternalAuthPage />, {
		route: `/external-auth/${provider}`,
		path: "/external-auth/:provider",
	});

describe("ExternalAuthPage", () => {
	// Regression test for the device flow: after a successful device-code
	// exchange the provider query must be invalidated so the page refetches and
	// flips to the authenticated view. Prior to the fix the dropped react-query
	// `onSuccess` left the UI stuck on the "Checking for authentication..."
	// polling screen until a manual refresh.
	it("refreshes the provider state after a successful device exchange", async () => {
		let exchanged = false;
		let providerRequests = 0;

		server.use(
			http.get(`/api/v2/external-auth/${provider}`, () => {
				providerRequests++;
				return HttpResponse.json<ExternalAuth>({
					...baseProvider,
					// The exchange marks the account authenticated. The page only
					// observes this after it invalidates and refetches the query.
					authenticated: exchanged,
				});
			}),
			http.get(`/api/v2/external-auth/${provider}/device`, () =>
				HttpResponse.json(deviceResponse),
			),
			http.post(`/api/v2/external-auth/${provider}/device`, () => {
				exchanged = true;
				return new HttpResponse(null, { status: 204 });
			}),
		);

		renderPage();

		// The polling screen renders first while the exchange is pending.
		await screen.findByText("Authenticate with GitHub");

		// Once the exchange succeeds, the provider query is invalidated and
		// refetched, so the authenticated view appears without a manual refresh.
		await waitFor(() => {
			expect(
				screen.getByText("You've authenticated with GitHub!"),
			).toBeInTheDocument();
		});

		// The provider endpoint is hit more than once: the initial load plus the
		// refetch triggered by the post-exchange invalidation.
		expect(providerRequests).toBeGreaterThan(1);
	});
});
