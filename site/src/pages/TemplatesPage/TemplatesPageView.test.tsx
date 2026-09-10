import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { MockTemplate } from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { TemplatesPageView } from "./TemplatesPageView";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ showOrganizations: false }),
}));

test("filters the templates list from the classic parameter warning", async () => {
	const filter: UseFilterResult = {
		query: "",
		update: vi.fn(),
		debounceUpdate: vi.fn(),
		cancelDebounce: vi.fn(),
		used: false,
		values: {},
	};
	const view = (
		<TemplatesPageView
			filterState={{ filter, menus: {} }}
			showOrganizations={false}
			canCreateTemplates={false}
			templateBuilderEnabled={false}
			examples={[]}
			templates={[
				{
					...MockTemplate,
					use_classic_parameter_flow: true,
				},
			]}
			templateUpdatePermissions={{
				[MockTemplate.organization_id]: true,
			}}
			workspacePermissions={{}}
		/>
	);
	const router = createMemoryRouter(
		[
			{ path: "/", element: view },
			{ path: "/templates", element: view },
		],
		{ initialEntries: ["/"] },
	);
	renderWithRouter(router);

	await userEvent.click(screen.getByRole("link", { name: "View templates" }));

	expect(router.state.location.pathname).toBe("/templates");
	expect(router.state.location.search).toBe(
		"?filter=use-classic-parameter-flow%3Atrue",
	);
});
