import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import type * as TypesGen from "#/api/typesGenerated";
import { AuthProvider } from "#/contexts/auth/AuthProvider";
import { getPreferredProxy } from "#/contexts/ProxyContext";
import { permissionChecks } from "#/modules/permissions";
import {
	MockAuthMethodsAll,
	MockPermissions,
	MockProxyLatencies,
	MockUserOwner,
	MockWorkspaceProxies,
} from "#/testHelpers/entities";
import { withDesktopViewport } from "#/testHelpers/storybook";
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
				<Story />
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
