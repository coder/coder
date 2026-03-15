import { MockTemplateVersion } from "testHelpers/entities";
import { renderWithRouter } from "testHelpers/renderHelpers";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { VersionsTable } from "./VersionsTable";

describe("TemplateVersionsPage", () => {
	it("uses explicit version links and keeps row actions independent", async () => {
		const user = userEvent.setup();
		const onPromoteClick = vi.fn();
		const router = createMemoryRouter(
			[
				{
					path: "/templates/:templateName/versions",
					element: (
						<VersionsTable
							activeVersionId="not-the-active-version"
							versions={[MockTemplateVersion]}
							onPromoteClick={onPromoteClick}
						/>
					),
				},
				{
					path: "/templates/:templateName/versions/:versionName",
					element: <div>Version details page</div>,
				},
			],
			{
				initialEntries: [
					`/templates/${MockTemplateVersion.template_id}/versions`,
				],
			},
		);

		renderWithRouter(router);

		const row = screen.getByTestId(`version-${MockTemplateVersion.id}`);
		expect(row).not.toHaveAttribute("role");
		expect(row).not.toHaveAttribute("tabindex");

		const versionLink = within(row).getByRole("link", {
			name: MockTemplateVersion.name,
		});
		expect(versionLink).toHaveAttribute(
			"href",
			`/templates/${MockTemplateVersion.template_id}/versions/${MockTemplateVersion.name}`,
		);

		await user.click(within(row).getByRole("button", { name: /promote/i }));
		expect(onPromoteClick).toHaveBeenCalledWith(MockTemplateVersion.id);
		expect(router.state.location.pathname).toBe(
			`/templates/${MockTemplateVersion.template_id}/versions`,
		);

		await user.click(versionLink);
		await waitFor(() => {
			expect(router.state.location.pathname).toBe(
				`/templates/${MockTemplateVersion.template_id}/versions/${MockTemplateVersion.name}`,
			);
		});
	});
});
