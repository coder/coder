import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type PathParams } from "msw";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockEntitlementsWithAIBridge,
	MockOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import ListSessionsPage from "./ListSessionsPage";

describe("ListSessionsPage", () => {
	beforeEach(() => {
		server.use(
			http.get("/api/v2/entitlements", () => {
				return HttpResponse.json(MockEntitlementsWithAIBridge);
			}),
		);
	});

	it("sends the selected organization to the API", async () => {
		server.use(
			http.get("/api/v2/organizations", () => {
				return HttpResponse.json([MockOrganization, MockOrganization2]);
			}),
			// The org menu only lists orgs where the user can read audit logs.
			// Grant every check so both orgs appear and the page stays visible.
			http.post<PathParams, TypesGen.AuthorizationRequest>(
				"/api/v2/authcheck",
				async ({ request }) => {
					const { checks } = await request.json();
					return HttpResponse.json(
						Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
					);
				},
			),
		);

		const spy = vi.spyOn(API, "getAIBridgeSessionList").mockResolvedValue({
			sessions: [],
			count: 0,
		});

		renderWithAuth(<ListSessionsPage />, {
			route: "/ai-gateway/sessions",
			path: "/ai-gateway/sessions",
		});
		await waitForLoaderToBeRemoved();

		const user = userEvent.setup();
		await user.click(
			screen.getByRole("button", { name: "Select an organization" }),
		);
		await user.click(
			// The option name includes the avatar initials, so match by substring.
			await screen.findByRole("option", { name: /My Organization 2/ }),
		);

		await waitFor(() =>
			expect(spy).toHaveBeenLastCalledWith(
				expect.objectContaining({
					q: expect.stringContaining("organization:my-organization-2"),
				}),
			),
		);
	});
});
