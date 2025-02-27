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
import OrganizationRedirect from "./OrganizationRedirect";

jest.spyOn(console, "error").mockImplementation(() => {});

const renderPage = async () => {
	const { router } = renderWithOrganizationSettingsLayout(
		<OrganizationRedirect />,
		{
			route: "/organizations",
			path: "/organizations",
			extraRoutes: [
				{
					path: "/organizations/:organization",
					element: <h1>Organization Settings</h1>,
				},
			],
		},
	);
	await waitForLoaderToBeRemoved();
	return router;
};

describe("OrganizationRedirect", () => {
	it("has no editable organizations", async () => {
		server.use(
			http.get("/api/v2/entitlements", () => {
				return HttpResponse.json(MockEntitlementsWithMultiOrg);
			}),
			http.get("/api/v2/organizations", () => {
				return HttpResponse.json([MockDefaultOrganization, MockOrganization2]);
			}),
			http.post("/api/v2/authcheck", async () => {
				return HttpResponse.json({});
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
					viewAnyMembers: true,
					[`${MockDefaultOrganization.id}.viewMembers`]: true,
					[`${MockDefaultOrganization.id}.editMembers`]: true,
					[`${MockOrganization2.id}.viewMembers`]: true,
					[`${MockOrganization2.id}.editMembers`]: true,
				});
			}),
		);
		const router = await renderPage();
		const form = screen.getByText("Organization Settings");
		expect(form).toBeInTheDocument();
		expect(router.state.location.pathname).toBe(
			`/organizations/${MockDefaultOrganization.name}`,
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
					viewAnyMembers: true,
					[`${MockDefaultOrganization.id}.viewMembers`]: true,
					[`${MockOrganization2.id}.viewMembers`]: true,
					[`${MockOrganization2.id}.editMembers`]: true,
				});
			}),
		);
		const router = await renderPage();
		const form = screen.getByText("Organization Settings");
		expect(form).toBeInTheDocument();
		expect(router.state.location.pathname).toBe(
			`/organizations/${MockOrganization2.name}`,
		);
	});
});
