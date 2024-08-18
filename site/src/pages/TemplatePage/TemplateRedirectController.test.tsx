import { waitFor } from "@testing-library/react";
import { API } from "api/api";
import * as M from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { TemplateRedirectController } from "./TemplateRedirectController";

const renderTemplateRedirectController = (route: string) => {
	return renderWithAuth(<TemplateRedirectController />, {
		route,
		path: "/templates/:organization?/:template",
	});
};

it("redirects from multi-org to single-org", async () => {
	const { router } = renderTemplateRedirectController(
		`/templates/${M.MockTemplate.organization_name}/${M.MockTemplate.name}`,
	);

	await waitFor(() =>
		expect(router.state.location.pathname).toEqual(
			`/templates/${M.MockTemplate.name}`,
		),
	);
});

it("redirects from single-org to multi-org", async () => {
	jest
		.spyOn(API, "getOrganizations")
		.mockResolvedValueOnce([M.MockDefaultOrganization, M.MockOrganization2]);

	const { router } = renderTemplateRedirectController(
		`/templates/${M.MockTemplate.name}`,
	);

	await waitFor(() =>
		expect(router.state.location.pathname).toEqual(
			`/templates/${M.MockDefaultOrganization.name}/${M.MockTemplate.name}`,
		),
	);
});
