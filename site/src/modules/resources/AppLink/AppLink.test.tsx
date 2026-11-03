import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import { SESSION_TOKEN_PLACEHOLDER } from "#/modules/apps/apps";
import { useAppLink } from "#/modules/apps/useAppLink";
import {
	MockBuildInfo,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceSubAgent,
} from "#/testHelpers/entities";
import { renderHookWithAuth } from "#/testHelpers/hooks";
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
		const trigger = await screen.findByText("Test App");
		expect(trigger).not.toHaveAttribute("href");
		await userEvent.click(trigger);
		expect(getApiKey).not.toHaveBeenCalled();
	});
});

describe("useAppLink", () => {
	it("trusts links to the access URL", async () => {
		const url = `vscode://coder.coder-remote/open?url=${new URL(MockBuildInfo.dashboard_url).origin}&token=${SESSION_TOKEN_PLACEHOLDER}`;
		const { result } = await renderHookWithAuth(
			() =>
				useAppLink(
					{ ...MockWorkspaceApp, external: true, url },
					{ workspace: MockWorkspace, agent: MockWorkspaceAgent },
				),
			{},
		);
		await waitFor(() => expect(result.current.isBlocked).toBe(false));
	});

	it("opens sub-agent apps without a session token", async () => {
		const getApiKey = vi.spyOn(API, "getApiKey");
		const url = `vscode://coder.coder-remote/open?url=${location.origin}&token=${SESSION_TOKEN_PLACEHOLDER}`;
		const { result } = await renderHookWithAuth(
			() =>
				useAppLink(
					{ ...MockWorkspaceApp, external: true, url },
					{ workspace: MockWorkspace, agent: MockWorkspaceSubAgent },
				),
			{},
		);
		expect(result.current.href).toBe(
			`vscode://coder.coder-remote/open?url=${location.origin}&token=`,
		);
		expect(result.current.hasToken).toBe(true);
		expect(getApiKey).not.toHaveBeenCalled();
	});
});
