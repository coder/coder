import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import {
	MockDefaultOrganization,
	MockEntitlementsWithMultiOrg,
	MockOrganization2,
} from "testHelpers/entities";
import {
	renderWithOrganizationSettingsLayout,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import OrganizationSettingsPage from "./OrganizationSettingsPage";

jest.spyOn(console, "error").mockImplementation(() => {});

const renderPage = async () => {
	renderWithOrganizationSettingsLayout(<OrganizationSettingsPage />, {
		route: "/organizations",
		path: "/organizations/:organization?",
	});
	await waitForLoaderToBeRemoved();
};

describe("OrganizationSettingsPage", () => {
	it("has no editable organizations", async () => {
		server.use(
			http.get("/api/v2/entitlements", () => {
				return HttpResponse.json(MockEntitlementsWithMultiOrg);
			}),
			http.get("/api/v2/organizations", () => {
				return HttpResponse.json([MockDefaultOrganization, MockOrganization2]);
			}),
			http.post("/api/v2/authcheck", async () => {
				return HttpResponse.json({
					viewDeploymentValues: true,
				});
			}),
		);
		await renderPage();
		await screen.findByText("No organizations found");
	});

	it("redirects to default organization", async () => {
		server.use(
			http.get("/api/v2/entitlements", () => {
				return HttpResponse.json(MockEntitlementsWithMultiOrg);
			}),
			http.get("/api/v2/organizations", () => {
				// Default always preferred regardless of order.
				return HttpResponse.json([MockOrganization2, MockDefaultOrganization]);
			}),
			http.post("/api/v2/authcheck", async () => {
				return HttpResponse.json({
					[`${MockDefaultOrganization.id}.editOrganization`]: true,
					[`${MockOrganization2.id}.editOrganization`]: true,
					viewDeploymentValues: true,
				});
			}),
		);
		await renderPage();
		const form = screen.getByTestId("org-settings-form");
		expect(within(form).getByRole("textbox", { name: "Slug" })).toHaveValue(
			MockDefaultOrganization.name,
		);
	});

	it("redirects to non-default organization", async () => {
		server.use(
			http.get("/api/v2/entitlements", () => {
				return HttpResponse.json(MockEntitlementsWithMultiOrg);
			}),
			http.get("/api/v2/organizations", () => {
				return HttpResponse.json([MockDefaultOrganization, MockOrganization2]);
			}),
			http.post("/api/v2/authcheck", async () => {
				return HttpResponse.json({
					[`${MockOrganization2.id}.editOrganization`]: true,
					viewDeploymentValues: true,
				});
			}),
		);
		await renderPage();
		const form = screen.getByTestId("org-settings-form");
		expect(within(form).getByRole("textbox", { name: "Slug" })).toHaveValue(
			MockOrganization2.name,
		);
	});
});
