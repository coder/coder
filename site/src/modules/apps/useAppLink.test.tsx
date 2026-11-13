import { waitFor } from "@testing-library/react";
import {
	MockBuildInfo,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceSubAgent,
} from "#/testHelpers/entities";
import { renderHookWithAuth } from "#/testHelpers/hooks";
import { SESSION_TOKEN_PLACEHOLDER } from "./apps";
import { useAppLink } from "./useAppLink";

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
	});
});
