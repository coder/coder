import { screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { MockPermissions } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { RequireWorkspaceAccess } from "./RequireWorkspaceAccess";

const renderSchedulePage = () => {
	renderWithAuth(<RequireWorkspaceAccess />, {
		path: "/settings",
		route: "/settings/schedule",
		children: [{ path: "schedule", element: <h1>Schedule</h1> }],
		extraRoutes: [{ path: "/settings/account", element: <h1>Account</h1> }],
	});
};

describe("RequireWorkspaceAccess", () => {
	it("renders the route when the user can read workspaces", async () => {
		renderSchedulePage();

		await screen.findByText("Schedule");
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

		renderSchedulePage();

		await screen.findByText("Account");
		expect(screen.queryByText("Schedule")).not.toBeInTheDocument();
	});
});
