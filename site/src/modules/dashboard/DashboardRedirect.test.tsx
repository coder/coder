import { screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { MockPermissions } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { DashboardRedirect } from "./DashboardRedirect";

const renderDashboardRedirect = () => {
	renderWithAuth(<DashboardRedirect />, {
		path: "/",
		route: "/",
		extraRoutes: [
			{ path: "/workspaces", element: <h1>Workspaces</h1> },
			{ path: "/settings/account", element: <h1>Account</h1> },
		],
	});
};

describe("DashboardRedirect", () => {
	it("redirects to workspaces when the user can read workspaces", async () => {
		renderDashboardRedirect();

		await screen.findByText("Workspaces");
	});

	it("redirects to the account page when the user cannot read workspaces", async () => {
		server.use(
			http.post("/api/v2/authcheck", () => {
				return HttpResponse.json({
					...MockPermissions,
					viewWorkspaces: false,
				});
			}),
		);

		renderDashboardRedirect();

		await screen.findByText("Account");
	});

	it("redirects to workspaces when the check is missing from the response", async () => {
		server.use(
			http.post("/api/v2/authcheck", () => {
				const { viewWorkspaces, ...rest } = MockPermissions;
				return HttpResponse.json(rest);
			}),
		);

		renderDashboardRedirect();

		await screen.findByText("Workspaces");
	});
});
