import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import { AppLink } from "./AppLink";

const renderAppLink = (app: typeof MockWorkspaceApp) => {
	return renderWithAuth(
		<AppLink app={app} workspace={MockWorkspace} agent={MockWorkspaceAgent} />,
	);
};

// Regression test for https://github.com/coder/coder/issues/18573:
// open_in="tab" was not opening links in a new tab.
describe("AppLink", () => {
	it("sets target=_blank and rel=noreferrer when open_in is tab", async () => {
		renderAppLink({ ...MockWorkspaceApp, open_in: "tab" });
		const link = await screen.findByRole("link");
		expect(link).toHaveAttribute("target", "_blank");
		expect(link).toHaveAttribute("rel", "noreferrer");
	});
});
