import "#/testHelpers/monaco";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { API } from "#/api/api";
import { networkPolicyKey } from "#/api/queries/networkPolicies";
import { MockOrganization, MockTemplate } from "#/testHelpers/entities";
import { MockNetworkPolicy } from "#/testHelpers/networkPolicies";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import NetworkPoliciesPage from "./NetworkPoliciesPage";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		experiments: ["network-policies"],
		organizations: [MockOrganization],
	}),
}));

it("focuses the destination after creation without moving focus on policy refresh", async () => {
	const create = vi
		.spyOn(API.experimental, "createNetworkPolicy")
		.mockResolvedValue(MockNetworkPolicy);
	const detail = vi
		.spyOn(API.experimental, "getNetworkPolicy")
		.mockResolvedValue(MockNetworkPolicy);
	vi.spyOn(API.experimental, "getNetworkPolicyTemplates").mockResolvedValue([
		MockTemplate,
	]);
	const basePath = "/organizations/default/network-policies";
	const { queryClient, router } = renderWithRouter(
		createMemoryRouter(
			[
				{
					path: "/organizations/:organization/network-policies/new",
					element: <NetworkPoliciesPage create />,
				},
				{
					path: "/organizations/:organization/network-policies/:policyId",
					element: <NetworkPoliciesPage />,
				},
			],
			{ initialEntries: [`${basePath}/new`] },
		),
	);
	const user = userEvent.setup();
	await user.type(
		await screen.findByRole("textbox", { name: /^Name/ }),
		MockNetworkPolicy.name,
	);
	await user.click(screen.getByRole("button", { name: "Create policy" }));
	await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
	await waitFor(() =>
		expect(router.state.location.pathname).toBe(
			`${basePath}/${MockNetworkPolicy.id}`,
		),
	);
	await waitFor(() =>
		expect(screen.getByRole("heading", { level: 1 })).toHaveFocus(),
	);
	const version = await screen.findByRole("combobox", { name: "Version" });
	await user.click(version);
	await user.keyboard("{Escape}");
	await act(async () => {
		await queryClient.invalidateQueries({
			queryKey: networkPolicyKey("default", MockNetworkPolicy.id),
		});
	});
	await waitFor(() => expect(detail).toHaveBeenCalledTimes(2));
	expect(version).toHaveFocus();
});
