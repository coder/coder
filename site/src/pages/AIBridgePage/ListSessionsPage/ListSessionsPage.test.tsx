import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http, type PathParams } from "msw";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockEntitlementsWithAIBridge,
	MockOrganization,
	MockOrganization2,
	MockSession,
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

	it("opens a cached session during a background refresh", async () => {
		const user = userEvent.setup();
		const response = { sessions: [MockSession], count: 1 };
		const refresh = createDeferred<TypesGen.AIBridgeListSessionsResponse>();
		const spy = vi
			.spyOn(API, "getAIBridgeSessionList")
			.mockResolvedValueOnce(response)
			.mockReturnValue(refresh.promise);
		const { router, queryClient } = renderWithAuth(<ListSessionsPage />, {
			route: "/ai-gateway/sessions",
			path: "/ai-gateway/sessions",
			extraRoutes: [{ path: "/ai-gateway/sessions/:sessionId", element: null }],
		});
		await screen.findByRole("row", { name: /can.*I really fix it/ });

		try {
			await act(async () => {
				void queryClient.refetchQueries({ type: "active" });
			});
			await waitFor(() => expect(spy).toHaveBeenCalledTimes(2));
			await user.click(
				screen.getByRole("row", { name: /can.*I really fix it/ }),
			);
			await waitFor(() =>
				expect(router.state.location.pathname).toBe(
					`/ai-gateway/sessions/${MockSession.id}`,
				),
			);
		} finally {
			await act(async () => refresh.resolve(response));
		}
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
		await user.click(screen.getByRole("button", { name: "Organization: All" }));
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
