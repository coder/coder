import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import {
	MockOrganization,
	MockOrganization2,
	MockTemplate,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import TemplatesPage from "./TemplatesPage";

describe("TemplatesPage", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

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

	it("filters by a readable organization the user is not a member of", async () => {
		vi.spyOn(API, "getOrganizations").mockResolvedValue([
			MockOrganization,
			MockOrganization2,
		]);
		vi.spyOn(API, "getMyOrganizations").mockResolvedValue([MockOrganization]);
		const user = userEvent.setup();
		const { router } = renderWithAuth(<TemplatesPage />, {
			route: "/templates",
			path: "/templates",
		});

		await user.click(
			await screen.findByRole("button", { name: "Select an organization" }),
		);
		await user.click(
			await screen.findByRole("option", {
				name: new RegExp(`${MockOrganization2.display_name}$`),
			}),
		);

		expect(
			new URLSearchParams(router.state.location.search).get("filter"),
		).toBe(`organization:${MockOrganization2.name}`);
	});
});
