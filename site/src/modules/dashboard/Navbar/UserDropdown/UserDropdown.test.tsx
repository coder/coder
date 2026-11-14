import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { withDefaultFeatures } from "#/api/api";
import { meAISpendKey } from "#/api/queries/users";
import {
	MockBuildInfo,
	MockEntitlements,
	MockMultiOrgExceededUserAISpendStatus,
	MockOrganization,
	MockOrganization2,
	MockUserOwner,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { UserDropdown } from "./UserDropdown";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		entitlements: {
			...MockEntitlements,
			has_license: true,
			features: withDefaultFeatures({
				aibridge: { enabled: true, entitlement: "entitled" },
			}),
		},
	}),
}));

const aiSpendPath = "/api/v2/users/me/ai/spend";

const renderOpenMenu = async (user: ReturnType<typeof userEvent.setup>) => {
	server.use(
		http.get(aiSpendPath, () =>
			HttpResponse.json(MockMultiOrgExceededUserAISpendStatus),
		),
	);
	const { queryClient } = render(
		<UserDropdown
			user={MockUserOwner}
			buildInfo={MockBuildInfo}
			supportLinks={[]}
			onSignOut={vi.fn()}
			canViewLicenses={false}
		/>,
	);
	await user.click(
		await screen.findByRole("button", {
			name: /User menu\. AI spend limit exceeded$/,
		}),
	);
	const menu = await screen.findByRole("menu");
	const orgTrigger = screen.getByRole("menuitem", {
		name: `Organization: ${MockOrganization2.display_name}`,
	});
	return { menu, orgTrigger, queryClient };
};

describe("UserDropdown AI spend", () => {
	it("switches the shown spend and avatar badge to the organization the user picks", async () => {
		const user = userEvent.setup();
		const { menu, orgTrigger } = await renderOpenMenu(user);

		expect(menu).toHaveTextContent("$1,250 / $1,200 USD");
		expect(menu).toHaveTextContent("Limit exceeded");
		expect(menu).toHaveTextContent(
			"AI requests are paused until July 1, 2026.",
		);

		await user.click(orgTrigger);
		await user.click(
			screen.getByRole("menuitemradio", {
				name: MockOrganization.display_name,
			}),
		);
		expect(menu).toHaveTextContent("$819 / $1,200 USD");
		expect(menu).not.toHaveTextContent("Limit exceeded");

		await user.keyboard("{Escape}");
		expect(
			screen.getByRole("button", { name: /User menu/ }),
		).toHaveAccessibleName(/User menu$/);
	});

	it("keeps the spend and the open selector when a refetch fails", async () => {
		const user = userEvent.setup();
		const { menu, orgTrigger, queryClient } = await renderOpenMenu(user);
		await user.click(orgTrigger);

		server.use(
			http.get(aiSpendPath, () =>
				HttpResponse.json(
					{ message: "AI spend is temporarily unavailable" },
					{ status: 500 },
				),
			),
		);
		await queryClient.refetchQueries({ queryKey: meAISpendKey });

		await vi.waitFor(() =>
			expect(menu).toHaveTextContent("AI spend is temporarily unavailable"),
		);
		expect(menu).toHaveTextContent("$1,250 / $1,200 USD");
		expect(orgTrigger).toHaveAttribute("aria-expanded", "true");
	});
});
