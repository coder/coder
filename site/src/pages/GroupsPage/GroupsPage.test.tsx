import { MockGroup } from "testHelpers/entities";
import { renderWithRouter } from "testHelpers/renderHelpers";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { GroupsPageView } from "./GroupsPageView";

describe("GroupsPage", () => {
	it("uses explicit links for group navigation without turning rows into buttons", async () => {
		const user = userEvent.setup();
		const router = createMemoryRouter(
			[
				{
					path: "/groups",
					element: (
						<GroupsPageView
							groups={[MockGroup]}
							canCreateGroup={false}
							groupsEnabled
						/>
					),
				},
				{
					path: "/groups/:groupName",
					element: <div>Group details page</div>,
				},
			],
			{ initialEntries: ["/groups"] },
		);

		renderWithRouter(router);

		const row = screen.getByTestId(`group-${MockGroup.id}`);
		expect(row).not.toHaveAttribute("role");
		expect(row).not.toHaveAttribute("tabindex");

		const groupLink = within(row).getByRole("link", {
			name: MockGroup.display_name || MockGroup.name,
		});
		expect(groupLink).toHaveAttribute("href", `/groups/${MockGroup.name}`);

		await user.click(groupLink);
		await waitFor(() => {
			expect(router.state.location.pathname).toBe(`/groups/${MockGroup.name}`);
		});
	});
});
