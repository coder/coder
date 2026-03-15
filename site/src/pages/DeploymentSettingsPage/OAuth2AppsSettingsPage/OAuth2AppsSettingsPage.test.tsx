import { MockOAuth2ProviderApps } from "testHelpers/entities";
import { renderWithRouter } from "testHelpers/renderHelpers";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

describe("OAuth2AppsSettingsPage", () => {
	it("uses explicit app links without assigning row button semantics", async () => {
		const user = userEvent.setup();
		const app = MockOAuth2ProviderApps[0];
		const router = createMemoryRouter(
			[
				{
					path: "/deployment/oauth2-provider/apps",
					element: (
						<OAuth2AppsSettingsPageView
							apps={[app]}
							isLoading={false}
							error={undefined}
							canCreateApp={false}
						/>
					),
				},
				{
					path: "/deployment/oauth2-provider/apps/:appId",
					element: <div>OAuth2 app details page</div>,
				},
			],
			{ initialEntries: ["/deployment/oauth2-provider/apps"] },
		);

		renderWithRouter(router);

		const row = screen.getByTestId(`app-${app.id}`);
		expect(row).not.toHaveAttribute("role");
		expect(row).not.toHaveAttribute("tabindex");

		const appLink = within(row).getByRole("link", { name: app.name });
		expect(appLink).toHaveAttribute(
			"href",
			`/deployment/oauth2-provider/apps/${app.id}`,
		);

		await user.click(appLink);
		await waitFor(() => {
			expect(router.state.location.pathname).toBe(
				`/deployment/oauth2-provider/apps/${app.id}`,
			);
		});
	});
});
