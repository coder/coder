import { MockEntitlements } from "testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { screen } from "@testing-library/react";
import { withDefaultFeatures } from "api/api";
import { HttpResponse, http } from "msw";
import UsersPage from "./UsersPage";

const MockEntitlementsWithAIGovernance = {
	...MockEntitlements,
	features: withDefaultFeatures({
		ai_governance_user_limit: {
			enabled: true,
			entitlement: "entitled",
		},
	}),
};

const renderPage = async () => {
	renderWithAuth(<UsersPage />, {
		route: "/users",
		path: "/users",
	});
	await waitForLoaderToBeRemoved();
};

describe("UsersPage", () => {
	it("shows the AI add-on column when AI governance is entitled", async () => {
		server.use(
			http.get("/api/v2/entitlements", () => {
				return HttpResponse.json(MockEntitlementsWithAIGovernance);
			}),
		);

		await renderPage();

		expect(
			screen.getByRole("columnheader", { name: "AI add-on" }),
		).toBeInTheDocument();
	});

	it("hides the AI add-on column when AI governance is not entitled", async () => {
		server.use(
			http.get("/api/v2/entitlements", () => {
				return HttpResponse.json(MockEntitlements);
			}),
		);

		await renderPage();

		expect(
			screen.queryByRole("columnheader", { name: "AI add-on" }),
		).not.toBeInTheDocument();
	});
});
