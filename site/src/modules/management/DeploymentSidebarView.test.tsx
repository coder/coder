import { screen } from "@testing-library/react";
import { MockBuildInfo, MockPermissions } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { DeploymentSidebarView } from "./DeploymentSidebarView";

const renderSidebar = (showOrganizations: boolean) =>
	render(
		<DeploymentSidebarView
			permissions={MockPermissions}
			showOrganizations={showOrganizations}
			hidePremiumTab={false}
			buildInfo={MockBuildInfo}
		/>,
	);

describe(DeploymentSidebarView.name, () => {
	it("announces that Groups moves to organization settings", async () => {
		renderSidebar(true);

		expect(
			await screen.findByRole("link", { name: /^Groups/ }),
		).toHaveAccessibleName(/^Groups\s*\(in organization settings\)$/);
	});

	it("names Groups plainly without organizations", async () => {
		renderSidebar(false);

		expect(
			await screen.findByRole("link", { name: /^Groups/ }),
		).toHaveAccessibleName("Groups");
	});
});
