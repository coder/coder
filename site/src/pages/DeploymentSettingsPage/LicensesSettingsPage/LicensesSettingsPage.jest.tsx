import { screen } from "@testing-library/react";
import type { Entitlements } from "api/typesGenerated";
import { HttpResponse, http } from "msw";
import { MockLicenseResponse } from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import LicensesSettingsPage from "./LicensesSettingsPage";

// Mock react-confetti to avoid canvas issues in tests
jest.mock("react-confetti", () => () => null);

// Entitlements response without user_limit feature
const mockEntitlementsWithoutUserLimit: Entitlements = {
	errors: [],
	warnings: [],
	has_license: true,
	trial: false,
	require_telemetry: false,
	refreshed_at: "2022-05-20T16:45:57.122Z",
	features: {} as Entitlements["features"],
};

test("renders without crashing when user_limit feature is missing", async () => {
	server.use(
		http.get("/api/v2/entitlements", () => {
			return HttpResponse.json(mockEntitlementsWithoutUserLimit);
		}),
		http.get("/api/v2/licenses", () => {
			return HttpResponse.json(MockLicenseResponse);
		}),
		http.get("/api/v2/insights/user-status-counts", () => {
			return HttpResponse.json({ active: [] });
		}),
	);

	renderWithAuth(<LicensesSettingsPage />);

	await screen.findByText("Manage licenses to unlock Premium features.");
});
