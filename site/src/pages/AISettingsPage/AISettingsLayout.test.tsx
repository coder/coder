import { screen, waitFor } from "@testing-library/react";
import { type FC, useEffect } from "react";
import { type QueryStatus, useQueryClient } from "react-query";
import { expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import { entitlementsQueryKey } from "#/api/queries/entitlements";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { MockEntitlements, MockOrganization } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import AISettingsLayout from "./AISettingsLayout";

const aiGatewayEntitlements = {
	...MockEntitlements,
	features: withDefaultFeatures({
		aibridge: { enabled: true, entitlement: "entitled" },
	}),
};

const mockSpendOrganization = () => {
	vi.spyOn(API, "getOrganizations").mockResolvedValue([MockOrganization]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[MockOrganization.id]: true,
	});
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [],
		providers: [],
		unsupported_providers: [],
	});
};

type DashboardSnapshot = {
	aiGatewayEnabled: boolean;
	entitlementsStatus: QueryStatus | undefined;
};

const DashboardProbe: FC<{
	onRender: (snapshot: DashboardSnapshot) => void;
}> = ({ onRender }) => {
	const { entitlements } = useDashboard();
	const entitlementsStatus =
		useQueryClient().getQueryState(entitlementsQueryKey)?.status;
	useEffect(() => {
		onRender({
			aiGatewayEnabled: entitlements.features.aibridge.enabled,
			entitlementsStatus,
		});
	});
	return null;
};

// Entitlements are cached for the whole session, so the gate must be
// refreshed on navigation for a license change to reach the sidebar without
// a full page load.
it("requests entitlements again on the next AI settings navigation", async () => {
	const getEntitlements = vi
		.spyOn(API, "getEntitlements")
		.mockResolvedValue(aiGatewayEntitlements);
	mockSpendOrganization();

	const { router } = renderWithAuth(<AISettingsLayout />, {
		path: "/ai/settings",
		route: "/ai/settings/spend",
		children: [
			{ path: "spend", element: <div /> },
			{ path: "models", element: <div /> },
		],
	});
	await screen.findByRole("link", { name: "User spend" });
	const requestsBeforeNavigation = getEntitlements.mock.calls.length;

	await router.navigate("/ai/settings/models");

	await waitFor(() =>
		expect(getEntitlements.mock.calls.length).toBeGreaterThan(
			requestsBeforeNavigation,
		),
	);
});

it("keeps serving the cached entitlements when the navigation refetch fails", async () => {
	const getEntitlements = vi
		.spyOn(API, "getEntitlements")
		.mockResolvedValue(aiGatewayEntitlements);
	mockSpendOrganization();
	const onRender = vi.fn<(snapshot: DashboardSnapshot) => void>();

	const { router } = renderWithAuth(<AISettingsLayout />, {
		path: "/ai/settings",
		route: "/ai/settings/spend",
		children: [
			{ path: "spend", element: <DashboardProbe onRender={onRender} /> },
			{ path: "models", element: <DashboardProbe onRender={onRender} /> },
		],
	});
	await screen.findByRole("link", { name: "User spend" });

	getEntitlements.mockRejectedValue(new Error("entitlements unavailable"));
	await router.navigate("/ai/settings/models");

	await waitFor(() =>
		expect(onRender).toHaveBeenLastCalledWith({
			aiGatewayEnabled: true,
			entitlementsStatus: "error",
		}),
	);
});
