import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import { SESSION_TOKEN_PLACEHOLDER } from "#/modules/apps/apps";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
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

	// slim-window apps are opened programmatically via onClick /
	// window.open(), so the anchor must not carry target or rel
	// attributes that would interfere with that flow.
	it("does not set target or rel for slim-window apps", async () => {
		renderAppLink({ ...MockWorkspaceApp, open_in: "slim-window" });
		const link = await screen.findByRole("link");
		expect(link).not.toHaveAttribute("target");
		expect(link).not.toHaveAttribute("rel");
	});

	it.each([
		`vscode://coder.coder-remote/open?url=https://attacker.example&token=${SESSION_TOKEN_PLACEHOLDER}`,
		`coder://open?url=${location.origin}&token=${SESSION_TOKEN_PLACEHOLDER}`,
	])("does not mint a session key for %s", async (url) => {
		const getApiKey = vi.spyOn(API, "getApiKey");
		renderAppLink({ ...MockWorkspaceApp, external: true, url });
		const button = await screen.findByRole("button", {
			name: MockWorkspaceApp.display_name,
		});
		expect(button).toBeDisabled();
		await userEvent.click(button);
		expect(getApiKey).not.toHaveBeenCalled();
	});
});
