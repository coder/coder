import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import {
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
	limit,
	permissions = MockPermissions,
	warnings,
}: {
	actual?: number;
	entitlement?: "entitled" | "grace_period" | "not_entitled";
	limit?: number;
	permissions?: typeof MockPermissions;
	warnings?: string[];
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
				},
			});
		}),
		http.post("/api/v2/authcheck", () => {
			return HttpResponse.json(permissions);
		}),
	);

	renderWithAuth(<DashboardLayout />, {
		children: [{ element: <h1>Test page</h1> }],
	});
	await waitForLoaderToBeRemoved();
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
	await screen.findByTestId("update-check-snackbar");
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
