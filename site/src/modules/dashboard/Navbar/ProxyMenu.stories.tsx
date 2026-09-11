import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import type * as TypesGen from "#/api/typesGenerated";
import { AuthProvider } from "#/contexts/auth/AuthProvider";
import {
	getPreferredProxy,
	ProxyProvider,
	useProxy,
	userSelectedProxyStorage,
} from "#/contexts/ProxyContext";
import { permissionChecks } from "#/modules/permissions";
import {
	MockAuthMethodsAll,
	MockHealthyWildWorkspaceProxy,
	MockPermissions,
	MockPrimaryWorkspaceProxy,
	MockProxyLatencies,
	MockUserOwner,
	MockWorkspaceProxies,
} from "#/testHelpers/entities";
import {
	withDashboardProvider,
	withDesktopViewport,
} from "#/testHelpers/storybook";
import { ProxyMenu } from "./ProxyMenu";

const buildProxies = (count: number): TypesGen.WorkspaceProxy[] => {
	const seedProxy = MockWorkspaceProxies[0];
	const proxies: TypesGen.WorkspaceProxy[] = [];

	for (let index = 0; index < count; index++) {
		const suffix = String(index + 1).padStart(12, "0");
		const id = `10000000-0000-4000-8000-${suffix}`;
		const isHealthy = index % 7 !== 0;

		proxies.push({
			...seedProxy,
			id,
			name: `region-${index + 1}`,
			display_name: `Region ${index + 1}`,
			healthy: isHealthy,
		});
	}

	return proxies;
};

const buildLatencies = (
	proxies: TypesGen.WorkspaceProxy[],
): typeof MockProxyLatencies => {
	const latencies: typeof MockProxyLatencies = {};

	for (const [index, proxy] of proxies.entries()) {
		if (!proxy.healthy) {
			continue;
		}

		latencies[proxy.id] = {
			accurate: true,
			latencyMS: 20 + index * 3,
			at: new Date(),
			nextHopProtocol: "h2",
		};
	}

	return latencies;
};

const manyProxies = buildProxies(45);

const defaultProxyContextValue = {
	latenciesLoaded: true,
	proxyLatencies: MockProxyLatencies,
	proxy: getPreferredProxy(MockWorkspaceProxies, undefined),
	proxies: MockWorkspaceProxies,
	isLoading: false,
	isFetched: true,
	setProxy: fn(),
	clearProxy: fn(),
	refetchProxyLatencies: () => new Date(),
};

const meta: Meta<typeof ProxyMenu> = {
	title: "modules/dashboard/ProxyMenu",
	component: ProxyMenu,
	args: {
		proxyContextValue: defaultProxyContextValue,
	},
	decorators: [
		(Story) => (
			<AuthProvider>
				<div className="flex justify-end">
					<Story />
				</div>
			</AuthProvider>
		),
		withDesktopViewport,
	],
	parameters: {
		queries: [
			{ key: ["me"], data: MockUserOwner },
			{ key: ["authMethods"], data: MockAuthMethodsAll },
			{ key: ["hasFirstUser"], data: true },
			{
				key: getAuthorizationKey({ checks: permissionChecks }),
				data: MockPermissions,
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ProxyMenu>;

export const Closed: Story = {};

export const ClosedWarningLatency: Story = {
	args: {
		proxyContextValue: {
			...defaultProxyContextValue,
			proxyLatencies: {
				...MockProxyLatencies,
				[MockWorkspaceProxies[0].id]: {
					accurate: true,
					latencyMS: 224,
					at: new Date(),
					nextHopProtocol: "h2",
				},
			},
		},
	},
};

export const ClosedCriticalLatency: Story = {
	args: {
		proxyContextValue: {
			...defaultProxyContextValue,
			proxyLatencies: {
				...MockProxyLatencies,
				[MockWorkspaceProxies[0].id]: {
					accurate: true,
					latencyMS: 471,
					at: new Date(),
					nextHopProtocol: "h2",
				},
			},
		},
	},
};

export const ClosedNoLatency: Story = {
	args: {
		proxyContextValue: {
			...defaultProxyContextValue,
			proxyLatencies: Object.fromEntries(
				Object.entries(MockProxyLatencies).filter(
					([id]) => id !== MockWorkspaceProxies[0].id,
				),
			),
		},
	},
};

export const Opened: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
	},
};

export const SingleProxy: Story = {
	args: {
		proxyContextValue: {
			...defaultProxyContextValue,
			proxies: [MockWorkspaceProxies[0]],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
	},
};

export const ManyProxiesOpened: Story = {
	args: {
		proxyContextValue: {
			...defaultProxyContextValue,
			proxies: manyProxies,
			proxyLatencies: buildLatencies(manyProxies),
			proxy: getPreferredProxy(manyProxies, undefined),
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
	},
};

const ConnectedProxyMenu = () => <ProxyMenu proxyContextValue={useProxy()} />;

export const PersistedSelection: Story = {
	render: () => (
		<ProxyProvider>
			<ConnectedProxyMenu />
		</ProxyProvider>
	),
	decorators: [withDashboardProvider],
	beforeEach: () => {
		const regions = [MockPrimaryWorkspaceProxy, MockHealthyWildWorkspaceProxy];
		spyOn(API, "getWorkspaceProxyRegions").mockResolvedValue({ regions });
		userSelectedProxyStorage.set(MockPrimaryWorkspaceProxy);
		localStorage.setItem(
			"workspace-proxy-latencies",
			JSON.stringify(
				Object.fromEntries(
					regions.map((region, index) => [
						region.id,
						[
							{
								...MockProxyLatencies[region.id],
								latencyMS: 10 + index,
								at: "2100-01-01T00:00:00.000Z",
							},
						],
					]),
				),
			),
		);
		return () => {
			userSelectedProxyStorage.remove();
			localStorage.removeItem("workspace-proxy-latencies");
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /Latency for Default/ }),
		);
		await userEvent.click(
			await screen.findByRole("menuitemradio", { name: /Subdomain Supported/ }),
		);
		expect(
			await canvas.findByRole("button", {
				name: /Latency for Subdomain Supported/,
			}),
		).toBeVisible();
		expect(userSelectedProxyStorage.get()?.id).toBe(
			MockHealthyWildWorkspaceProxy.id,
		);

		userSelectedProxyStorage.set(MockPrimaryWorkspaceProxy);
		expect(
			await canvas.findByRole("button", { name: /Latency for Default/ }),
		).toBeVisible();

		const next = JSON.stringify(MockHealthyWildWorkspaceProxy);
		localStorage.setItem("user-selected-proxy", next);
		window.dispatchEvent(
			new StorageEvent("storage", {
				key: "user-selected-proxy",
				newValue: next,
				storageArea: localStorage,
			}),
		);
		expect(
			await canvas.findByRole("button", {
				name: /Latency for Subdomain Supported/,
			}),
		).toBeVisible();
	},
};
