import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { deploymentSSHConfigQueryKey } from "#/api/queries/deployment";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import {
	getPreferredProxy,
	ProxyContext,
	type ProxyContextValue,
} from "#/contexts/ProxyContext";
import {
	MockDeploymentSSH,
	MockProxyLatencies,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { WorkspacePill } from "./WorkspacePill";

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

const renderPill = () => {
	const queryClient = createTestQueryClient();
	queryClient.setQueryData(deploymentSSHConfigQueryKey, MockDeploymentSSH);

	const Wrapper: FC<PropsWithChildren> = ({ children }) => (
		<QueryClientProvider client={queryClient}>
			<ProxyContext.Provider value={proxyContextValue}>
				<TooltipProvider>
					<MemoryRouter>{children}</MemoryRouter>
				</TooltipProvider>
			</ProxyContext.Provider>
		</QueryClientProvider>
	);

	return render(
		<WorkspacePill
			workspace={MockWorkspace}
			agent={{ ...MockWorkspaceAgent, display_apps: [], apps: [] }}
			chatId="chat-1"
		/>,
		{ wrapper: Wrapper },
	);
};

describe("WorkspacePill", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("copies the SSH command built from the deployment hostname suffix", async () => {
		const writeText = vi.fn().mockResolvedValue(undefined);
		vi.stubGlobal("navigator", { clipboard: { writeText } });

		renderPill();
		await userEvent.click(
			screen.getByRole("button", {
				name: `${MockWorkspace.name} workspace menu`,
			}),
		);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "Copy SSH Command" }),
		);

		expect(writeText).toHaveBeenCalledWith(
			`ssh ${MockWorkspaceAgent.name}.${MockWorkspace.name}.${MockWorkspace.owner_name}.${MockDeploymentSSH.hostname_suffix}`,
		);
	});
});
