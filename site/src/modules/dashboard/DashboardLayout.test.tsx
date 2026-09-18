import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { RouteObject } from "react-router";
import type { AuthorizationRequest, Entitlements } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockEntitlements,
	MockNoPermissions,
	MockPermissions,
} from "#/testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { DashboardLayout } from "./DashboardLayout";

const renderDashboardLayout = async ({
	actual,
	entitlement = "entitled",
	features,
	limit,
	permissions = MockPermissions,
	organizationChecks = false,
	warnings,
	children = [{ element: <h1>Test page</h1> }],
}: {
	actual?: number;
	entitlement?: "entitled" | "grace_period" | "not_entitled";
	features?: Partial<Entitlements["features"]>;
	limit?: number;
	permissions?: typeof MockPermissions;
	/** Answer for authorization checks keyed by organization id. */
	organizationChecks?: boolean;
	warnings?: string[];
	children?: RouteObject[];
}) => {
	server.use(
		http.get("/api/v2/entitlements", () => {
			return HttpResponse.json({
				...MockEntitlements,
				warnings: warnings ?? MockEntitlements.warnings,
				has_license: true,
				refreshed_at: new Date().toISOString(),
				features: {
					...MockEntitlements.features,
					ai_governance_user_limit: {
						entitlement,
						enabled: true,
						...(actual !== undefined ? { actual } : {}),
						...(limit !== undefined ? { limit } : {}),
					},
					...features,
				},
			});
		}),
		// The inbox request fires once the permission queries settle and has no
		// default handler.
		http.get("/api/v2/notifications/inbox", () => {
			return HttpResponse.json({ notifications: [], unread_count: 0 });
		}),
		http.post("/api/v2/authcheck", async ({ request }) => {
			const { checks } = (await request.json()) as AuthorizationRequest;
			return HttpResponse.json(
				MockDefaultOrganization.id in checks
					? Object.fromEntries(
							Object.keys(checks).map((id) => [id, organizationChecks]),
						)
					: permissions,
			);
		}),
	);

	const result = renderWithAuth(<DashboardLayout />, { children });
	await waitForLoaderToBeRemoved();
	return result;
};

test("Show the new Coder version notification", async () => {
	server.use(
		http.get("/api/v2/updatecheck", () => {
			return HttpResponse.json({
				current: false,
				version: "v0.12.9",
				url: "https://github.com/coder/coder/releases/tag/v0.12.9",
			});
		}),
	);
	renderWithAuth(<DashboardLayout />, {
		children: [{ element: <h1>Test page</h1> }],
	});
	await screen.findByTestId("update-check-notice");
});

test("hides AI Governance seat warnings for non-admin users", async () => {
	await renderDashboardLayout({
		actual: 110,
		limit: 100,
		permissions: MockNoPermissions,
	});

	expect(
		screen.queryByText(/AI Governance add-on seats/),
	).not.toBeInTheDocument();
});

test("shows AI Governance over-limit warning in LicenseBanner for admin users", async () => {
	await renderDashboardLayout({
		actual: 110,
		limit: 100,
		permissions: MockPermissions,
	});

	expect(
		screen.getByText(
			/110 of 100 AI Governance add-on seats \(10 over the limit\)/,
		),
	).toBeInTheDocument();
});

test("navigates an organization group member reader to AI settings from Admin settings", async () => {
	const { router } = await renderDashboardLayout({
		permissions: MockNoPermissions,
		features: { aibridge: { enabled: true, entitlement: "entitled" } },
		organizationChecks: true,
		children: [{ path: "/ai/settings", element: <h1>AI settings</h1> }],
	});

	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("button", { name: "Admin settings" }),
	);
	await user.click(await screen.findByRole("menuitem", { name: "AI" }));
	await screen.findByRole("heading", { name: "AI settings" });
	expect(router.state.location.pathname).toBe("/ai/settings");
});

test("renders a skip link before navigation content", async () => {
	renderWithAuth(<DashboardLayout />, {
		children: [{ element: <h1>Test page</h1> }],
	});
	await waitForLoaderToBeRemoved();

	const skipToContentLink = screen.getByRole("link", {
		name: "Skip to main content",
	});
	const navigation = screen.getAllByRole("navigation")[0];
	const mainContent = document.getElementById("main-content");

	expect(skipToContentLink).toHaveAttribute("href", "#main-content");
	expect(mainContent).toHaveAttribute("tabindex", "-1");
	expect(
		skipToContentLink.compareDocumentPosition(navigation) &
			Node.DOCUMENT_POSITION_FOLLOWING,
	).toBeTruthy();
});

test("moves focus to main content when skip link is clicked", async () => {
	renderWithAuth(<DashboardLayout />, {
		children: [{ element: <h1>Test page</h1> }],
	});
	await waitForLoaderToBeRemoved();

	const user = userEvent.setup();
	const skipToContentLink = screen.getByRole("link", {
		name: "Skip to main content",
	});
	const mainContent = document.getElementById("main-content");

	expect(mainContent).not.toBeNull();
	await user.click(skipToContentLink);
	expect(mainContent).toHaveFocus();
});
