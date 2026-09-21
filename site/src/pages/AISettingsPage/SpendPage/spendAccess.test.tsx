import { renderHook, waitFor } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { afterEach, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import { aiSpendOrganizations } from "#/api/queries/aiBridge";
import { MockEntitlements, MockOrganization } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { useCanViewAISpend } from "./spendAccess";

const aiGatewayEntitlements = (enabled: boolean) => ({
	...MockEntitlements,
	features: withDefaultFeatures({
		aibridge: { enabled, entitlement: "entitled" },
	}),
});
const dashboard = { entitlements: aiGatewayEntitlements(true) };

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ entitlements: dashboard.entitlements }),
}));

afterEach(() => {
	dashboard.entitlements = aiGatewayEntitlements(true);
});

// The organizations query keeps its cached result once disabled, so access
// must follow the entitlement itself rather than the query being enabled.
it("revokes access once AI Gateway is disabled after the organizations loaded", async () => {
	vi.spyOn(API, "getOrganizations").mockResolvedValue([MockOrganization]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[MockOrganization.id]: true,
	});
	const queryClient = createTestQueryClient();
	const wrapper: FC<PropsWithChildren> = ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	const { result, rerender } = renderHook(() => useCanViewAISpend(), {
		wrapper,
	});
	await waitFor(() => expect(result.current.canView).toBe(true));

	dashboard.entitlements = aiGatewayEntitlements(false);
	rerender();

	expect(queryClient.getQueryData(aiSpendOrganizations().queryKey)).toEqual([
		MockOrganization,
	]);
	expect(result.current.canView).toBe(false);
});
