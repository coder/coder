import { MockTemplate } from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { TemplateFilterState } from "./TemplatesPage";
import { TemplatesPageView } from "./TemplatesPageView";

vi.mock("./TemplatesFilter", () => ({
	TemplatesFilter: () => null,
}));

describe("TemplatesPage", () => {
	it("uses explicit template links and keeps row actions independent", async () => {
		const user = userEvent.setup();
		const filterState = {
			filter: {} as TemplateFilterState["filter"],
			menus: {},
		} satisfies TemplateFilterState;

		const { router } = renderWithAuth(
			<TemplatesPageView
				error={undefined}
				filterState={filterState}
				showOrganizations={false}
				canCreateTemplates={false}
				examples={[]}
				templates={[MockTemplate]}
				workspacePermissions={{
					[MockTemplate.organization_id]: {
						createWorkspaceForUserID: true,
					},
				}}
			/>,
			{
				path: "/templates",
				route: "/templates",
				extraRoutes: [
					{
						path: "/templates/:templateName",
						element: <div>Template details page</div>,
					},
					{
						path: "/templates/:templateName/workspace",
						element: <div>Create workspace page</div>,
					},
				],
			},
		);

		const row = await screen.findByTestId(`template-${MockTemplate.id}`);
		expect(row).not.toHaveAttribute("role");
		expect(row).not.toHaveAttribute("tabindex");

		const templateLink = within(row).getByRole("link", {
			name: MockTemplate.display_name,
		});
		expect(templateLink).toHaveAttribute(
			"href",
			`/templates/${MockTemplate.name}`,
		);

		const createWorkspaceLink = within(row).getByRole("link", {
			name: /create workspace/i,
		});
		await user.click(createWorkspaceLink);
		await waitFor(() => {
			expect(router.state.location.pathname).toBe(
				`/templates/${MockTemplate.name}/workspace`,
			);
		});

		await router.navigate("/templates");
		await waitFor(() => {
			expect(router.state.location.pathname).toBe("/templates");
		});

		const templateLinkAfterReturn = within(
			await screen.findByTestId(`template-${MockTemplate.id}`),
		).getByRole("link", {
			name: MockTemplate.display_name,
		});
		await user.click(templateLinkAfterReturn);
		await waitFor(() => {
			expect(router.state.location.pathname).toBe(
				`/templates/${MockTemplate.name}`,
			);
		});
	});
});
