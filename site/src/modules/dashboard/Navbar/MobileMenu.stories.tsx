import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { expect, fn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockPrimaryWorkspaceProxy,
	MockProxyLatencies,
	MockSupportLinks,
	MockUserMember,
	MockUserOwner,
	MockWorkspaceProxies,
} from "#/testHelpers/entities";
import { MobileMenu } from "./MobileMenu";

const defaultProxyContextValue = {
	latenciesLoaded: true,
	proxy: {
		preferredPathAppURL: "",
		preferredWildcardHostname: "",
		proxy: MockPrimaryWorkspaceProxy,
	},
	isLoading: false,
	isFetched: true,
	setProxy: fn(),
	clearProxy: fn(),
	refetchProxyLatencies: fn(),
	proxyLatencies: MockProxyLatencies,
	proxies: MockWorkspaceProxies,
};

const meta: Meta<typeof MobileMenu> = {
	title: "modules/dashboard/MobileMenu",
	parameters: {
		layout: "fullscreen",
		viewport: {
			defaultViewport: "iphone12",
		},
	},
	component: MobileMenu,
	args: {
		proxyContextValue: defaultProxyContextValue,
		user: MockUserOwner,
		supportLinks: MockSupportLinks,
		onSignOut: fn(),
		isDefaultOpen: true,
		canViewWorkspaces: true,
		canViewTemplates: true,
		canViewModels: false,
		adminPermissions: {
			canViewDeployment: true,
			canViewOrganizations: true,
			canViewAISettings: true,
			canViewAuditLog: true,
			canViewConnectionLog: true,
			canViewAIBridge: true,
			canViewHealth: true,
		},
	},
	decorators: [withNavbarMock],
};

export default meta;
type Story = StoryObj<typeof MobileMenu>;

export const Closed: Story = {
	args: {
		isDefaultOpen: false,
	},
};

export const Admin: Story = {
	play: openAdminSettings,
};

export const Auditor: Story = {
	args: {
		user: MockUserMember,
		adminPermissions: {
			canViewAuditLog: true,
		},
	},
	play: openAdminSettings,
};

export const OrgAdmin: Story = {
	args: {
		user: MockUserMember,
		adminPermissions: {
			canViewAuditLog: true,
			canViewOrganizations: true,
		},
	},
	play: openAdminSettings,
};

export const Member: Story = {
	args: {
		user: MockUserMember,
		adminPermissions: {},
	},
};

export const WithoutWorkspaceAccess: Story = {
	args: {
		user: MockUserMember,
		adminPermissions: {},
		canViewWorkspaces: false,
		canViewTemplates: false,
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		await body.findByText("Workspaces");

		expect(
			body.queryByRole("link", { name: "Workspaces" }),
		).not.toBeInTheDocument();
		expect(
			body.queryByRole("menuitem", { name: /workspace proxy settings/i }),
		).not.toBeInTheDocument();

		// The reason is revealed on tap rather than always rendered.
		expect(
			body.queryByText(/workspaces are not available/i),
		).not.toBeInTheDocument();
		const item = body.getByRole("menuitem", {
			name: "Workspaces (unavailable)",
		});
		// Radix removes items marked with its own `disabled` prop from roving
		// focus, which puts the message out of keyboard reach.
		expect(item).not.toHaveAttribute("data-disabled");

		await user.click(item);
		await body.findByText(/workspaces are not available/i);
		expect(item).toHaveAttribute("aria-expanded", "true");
	},
};

export const MemberWithModelAccess: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/" },
			routing: [
				{ path: "/", useStoryElement: true },
				{
					path: "/ai/settings/models",
					element: <h1>Organization models</h1>,
				},
			],
		}),
	},
	args: {
		user: MockUserMember,
		adminPermissions: {},
		canViewModels: true,
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await expect(
			body.queryByRole("menuitem", { name: /admin settings/i }),
		).not.toBeInTheDocument();
		await user.click(body.getByRole("menuitem", { name: "Models" }));
		await expect(
			await canvas.findByRole("heading", { name: "Organization models" }),
		).toBeInTheDocument();
	},
};

export const ProxySettings: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const menuItem = await body.findByRole("menuitem", {
			name: /workspace proxy settings/i,
		});
		await user.click(menuItem);
	},
};

export const ProxyWarningLatency: Story = {
	args: {
		proxyContextValue: {
			...defaultProxyContextValue,
			proxyLatencies: {
				...MockProxyLatencies,
				[MockPrimaryWorkspaceProxy.id]: {
					accurate: true,
					latencyMS: 224,
					at: new Date(),
					nextHopProtocol: "h2",
				},
			},
		},
	},
};

export const ProxyCriticalLatency: Story = {
	args: {
		proxyContextValue: {
			...defaultProxyContextValue,
			proxyLatencies: {
				...MockProxyLatencies,
				[MockPrimaryWorkspaceProxy.id]: {
					accurate: true,
					latencyMS: 471,
					at: new Date(),
					nextHopProtocol: "h2",
				},
			},
		},
	},
};

export const ProxyNoLatency: Story = {
	args: {
		proxyContextValue: {
			...defaultProxyContextValue,
			proxyLatencies: Object.fromEntries(
				Object.entries(MockProxyLatencies).filter(
					([id]) => id !== MockPrimaryWorkspaceProxy.id,
				),
			),
		},
	},
};

export const UserSettings: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const menuItem = await body.findByRole("menuitem", {
			name: /user settings/i,
		});
		await user.click(menuItem);
	},
};

function withNavbarMock(Story: FC) {
	return (
		<div className="h-[72px] border-0 border-b border-solid px-6 flex items-center justify-end">
			<Story />
		</div>
	);
}

async function openAdminSettings({
	canvasElement,
}: {
	canvasElement: HTMLElement;
}) {
	const user = userEvent.setup();
	const body = within(canvasElement.ownerDocument.body);
	const menuItem = await body.findByRole("menuitem", {
		name: /admin settings/i,
	});
	await user.click(menuItem);
}
