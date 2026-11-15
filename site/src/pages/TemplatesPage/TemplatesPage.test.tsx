import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockTemplate } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import TemplatesPage from "./TemplatesPage";

describe("TemplatesPage", () => {
	it("opens a template from its name link with the keyboard", async () => {
		const user = userEvent.setup();
		const { router } = renderWithAuth(<TemplatesPage />, {
			route: "/templates",
			path: "/templates",
		});

		const link = await screen.findByRole("link", {
			name: MockTemplate.display_name,
		});
		link.focus();
		await user.keyboard("{Enter}");

		expect(router.state.location.pathname).toMatch(
			new RegExp(`/templates/(.+/)?${MockTemplate.name}$`),
		);
	});
});
