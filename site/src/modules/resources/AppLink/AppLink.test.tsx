import { screen } from "@testing-library/react";
import { ProxyProvider } from "contexts/ProxyContext";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { AppLink } from "./AppLink";

const renderAppLink = (app: typeof MockWorkspaceApp) => {
	return renderWithAuth(
		<ProxyProvider>
			<AppLink app={app} workspace={MockWorkspace} agent={MockWorkspaceAgent} />
		</ProxyProvider>,
	);
};

// Regression tests for https://github.com/coder/coder/issues/18573:
// open_in="tab" was not opening links in a new tab.
describe("AppLink", () => {
	it("sets target=_blank and rel=noopener noreferrer when open_in is tab", async () => {
		renderAppLink({ ...MockWorkspaceApp, open_in: "tab" });
		const link = await screen.findByRole("link");
		expect(link).toHaveAttribute("target", "_blank");
		expect(link).toHaveAttribute("rel", "noopener noreferrer");
	});

	// slim-window uses window.open() in onClick rather than a target attribute,
	// so the anchor must not get target="_blank" or rel to avoid double-opening.
	it("does not set target or rel when open_in is slim-window", async () => {
		renderAppLink({ ...MockWorkspaceApp, open_in: "slim-window" });
		const link = await screen.findByRole("link");
		expect(link).not.toHaveAttribute("target");
		expect(link).not.toHaveAttribute("rel");
	});
});
