import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { API } from "#/api/api";
import { MockOrganization, MockTemplate } from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { NetworkPoliciesNavigation } from "./NetworkPoliciesNavigation";

it.each([
	["organization admin", true, false],
	["template ACL admin", false, false],
	["template ACL admin after retry", false, true],
])(
	"opens native policy routes in a single organization for an %s",
	async (_role, hasOrganizationAccess, retry) => {
		const templates = vi
			.spyOn(API.experimental, "getNetworkPolicyTemplates")
			.mockResolvedValue(hasOrganizationAccess ? [] : [MockTemplate]);
		if (retry)
			templates.mockRejectedValueOnce({ message: "Access lookup failed" });
		const router = createMemoryRouter(
			[
				{
					path: "/templates",
					element: (
						<NetworkPoliciesNavigation
							organizations={[MockOrganization]}
							templateUpdatePermissions={{
								[MockOrganization.id]: hasOrganizationAccess,
							}}
						/>
					),
				},
				{
					path: "/organizations/:organization/network-policies",
					element: <div />,
				},
			],
			{ initialEntries: ["/templates"] },
		);
		renderWithRouter(router);
		const user = userEvent.setup();
		if (retry) {
			await user.click(
				await screen.findByRole("button", {
					name: `Retry network policies for ${MockOrganization.display_name || MockOrganization.name}`,
				}),
			);
			await waitFor(() => expect(templates).toHaveBeenCalledTimes(2));
		}
		await user.click(
			await screen.findByRole("link", { name: "Network policies" }),
		);
		await waitFor(() =>
			expect(router.state.location.pathname).toBe(
				`/organizations/${MockOrganization.name}/network-policies`,
			),
		);
	},
);
