import {
	MockProxyLatencies,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "testHelpers/entities";
import { render } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import type { ProxyContextValue } from "contexts/ProxyContext";
import { getPreferredProxy, ProxyContext } from "contexts/ProxyContext";
import { AppLink } from "./AppLink";

function renderAppLink(
	agentOverrides: Partial<typeof MockWorkspaceAgent> = {},
	appOverrides: Partial<typeof MockWorkspaceApp> = {},
) {
	const agent = { ...MockWorkspaceAgent, ...agentOverrides };
	const app = { ...MockWorkspaceApp, ...appOverrides };

	const proxyContextValue: ProxyContextValue = {
		latenciesLoaded: true,
		proxyLatencies: MockProxyLatencies,
		proxy: getPreferredProxy([], undefined),
		proxies: [],
		isLoading: false,
		isFetched: true,
		setProxy: () => {},
		clearProxy: () => {},
		refetchProxyLatencies: () => new Date(),
	};

	return render(
		<ProxyContext.Provider value={proxyContextValue}>
			<AppLink workspace={MockWorkspace} app={app} agent={agent} />
		</ProxyContext.Provider>,
	);
}

describe("AppLink", () => {
	it("is clickable when agent is connected", () => {
		renderAppLink({ status: "connected" });

		const link = screen.getByRole("link");
		expect(link).toHaveAttribute("href");
	});

	it("is not clickable when agent is connecting", () => {
		renderAppLink({ status: "connecting" });

		const link = screen.getByText(MockWorkspaceApp.display_name!);
		expect(link.closest("a")).not.toHaveAttribute("href");
	});

	it("is not clickable when agent is disconnected", () => {
		renderAppLink({ status: "disconnected" });

		const link = screen.getByText(MockWorkspaceApp.display_name!);
		expect(link.closest("a")).not.toHaveAttribute("href");
	});

	it("is not clickable when agent has timed out", () => {
		renderAppLink({ status: "timeout" });

		const link = screen.getByText(MockWorkspaceApp.display_name!);
		expect(link.closest("a")).not.toHaveAttribute("href");
	});

	it("is not clickable when agent has blocking startup script running", () => {
		renderAppLink({
			status: "connected",
			lifecycle_state: "starting",
			startup_script_behavior: "blocking",
		});

		const link = screen.getByText(MockWorkspaceApp.display_name!);
		expect(link.closest("a")).not.toHaveAttribute("href");
	});
});
